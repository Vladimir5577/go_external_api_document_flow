// Package pdf — порт CitizenAppealPdfService и ContractApplicationPdfService
// из монолита. Стек подобран парный: TCPDF+FPDI в PHP ↔ fpdf+gofpdi здесь,
// поэтому примитивы вёрстки переносятся один в один.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go_external_api_document_flow/internal/dto"

	"github.com/go-pdf/fpdf"
	"github.com/go-pdf/fpdf/contrib/gofpdi"
)

const (
	fontFamily   = "dejavu"
	marginX      = 15.0
	marginTop    = 15.0
	marginBottom = 15.0
	labelWidth   = 55.0
	valueWidth   = 125.0
	lineHeight   = 6.0

	// PDF меряет страницы в пунктах, документ собираем в миллиметрах
	ptToMm = 25.4 / 72
)

// imageExtensions — те же расширения, что в IMAGE_EXTENSIONS монолита.
// webp и bmp сюда входят, но stdlib их не декодирует — они уйдут в заглушку.
var imageExtensions = map[string]bool{
	"jpeg": true, "jpg": true, "png": true, "gif": true, "webp": true, "bmp": true,
}

// Row — строка «метка → значение». Порядок важен, поэтому срез, а не map.
type Row struct {
	Label string
	Value string
}

// Attachment — вложение, уже скачанное из don_snab.
// Err заполняется, если скачать не удалось: вместо файла в PDF уйдёт заглушка.
type Attachment struct {
	Name      string
	SizeLabel string
	Content   []byte
	Err       error
}

// maxConcurrentDocuments ограничивает число одновременных генераций.
// Вложения (обычно 5–10 файлов по 3–7 МБ) держатся в памяти целиком вместе
// с собираемым документом, так что без потолка десяток параллельных выгрузок
// выест память контейнера и уронит заодно все остальные модули.
// В монолите этот потолок бесплатно задавало число воркеров PHP-FPM.
const maxConcurrentDocuments = 4

type Renderer struct {
	regular   []byte
	bold      []byte
	presenter *dto.Presenter
	slots     chan struct{}
}

// NewRenderer читает шрифты один раз на старте: без кириллического TTF
// fpdf умеет только латиницу, а стандартные шрифты PDF кириллицу не содержат.
func NewRenderer(regularPath, boldPath string, presenter *dto.Presenter) (*Renderer, error) {
	regular, err := os.ReadFile(regularPath)
	if err != nil {
		return nil, fmt.Errorf("шрифт для PDF не найден: %w", err)
	}

	bold, err := os.ReadFile(boldPath)
	if err != nil {
		return nil, fmt.Errorf("жирный шрифт для PDF не найден: %w", err)
	}

	return &Renderer{
		regular:   regular,
		bold:      bold,
		presenter: presenter,
		slots:     make(chan struct{}, maxConcurrentDocuments),
	}, nil
}

// acquire занимает слот генерации. Ждём в очереди, но не дольше, чем живёт
// запрос: отменённому клиенту документ уже не нужен.
func (r *Renderer) acquire(ctx context.Context) error {
	select {
	case r.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Renderer) release() {
	<-r.slots
}

type doc struct {
	pdf      *fpdf.Fpdf
	imp      *gofpdi.Importer
	sources  []*io.ReadSeeker
	imageSeq int
}

func (r *Renderer) newDoc(title string) *doc {
	p := fpdf.New("P", "mm", "A4", "")
	p.SetCreator("document_flow", true)
	p.SetTitle(title, true)
	p.SetMargins(marginX, marginTop, marginX)
	p.SetAutoPageBreak(true, marginBottom)
	p.AddUTF8FontFromBytes(fontFamily, "", r.regular)
	p.AddUTF8FontFromBytes(fontFamily, "B", r.bold)
	p.AddPage()

	return &doc{pdf: p, imp: gofpdi.NewImporter()}
}

func (d *doc) output() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *doc) title(title string) {
	d.pdf.SetFont(fontFamily, "B", 16)
	d.pdf.SetTextColor(0, 0, 0)
	d.pdf.MultiCell(0, 10, title, "", "L", false)
	d.pdf.Ln(2)
}

// section рисует заголовок с линейкой и таблицу строк.
// Секция целиком пропускается, если все значения пустые, — как array_filter в PHP.
func (d *doc) section(heading string, rows []Row) {
	rows = nonEmpty(rows)
	if len(rows) == 0 {
		return
	}

	d.pdf.Ln(2)
	d.pdf.SetFont(fontFamily, "B", 12)
	d.pdf.SetTextColor(0, 0, 0)
	d.pdf.MultiCell(0, 7, heading, "", "L", false)

	pageWidth, _ := d.pdf.GetPageSize()
	d.pdf.SetDrawColor(200, 200, 200)
	y := d.pdf.GetY()
	d.pdf.Line(marginX, y, pageWidth-marginX, y)
	d.pdf.Ln(1)

	d.rows(rows)
}

func (d *doc) rows(rows []Row) {
	for _, row := range nonEmpty(rows) {
		d.row(row)
	}
}

// row верстает две колонки. В отличие от TCPDF у MultiCell здесь нет параметра ln,
// поэтому высоту строки считаем заранее и сами двигаем курсор.
func (d *doc) row(row Row) {
	d.pdf.SetFont(fontFamily, "", 10)

	_, pageHeight := d.pdf.GetPageSize()
	usable := pageHeight - marginTop - marginBottom

	lines := len(d.pdf.SplitText(row.Value, valueWidth))
	if labelLines := len(d.pdf.SplitText(row.Label, labelWidth)); labelLines > lines {
		lines = labelLines
	}
	height := float64(lines) * lineHeight

	// Значение длиннее страницы (например, текст обращения) в две колонки
	// не помещается: fpdf при переносе страницы сбрасывает X на левое поле
	// и правая колонка уехала бы влево. Такие блоки печатаем во всю ширину.
	if height > usable {
		if row.Label != "" {
			d.pdf.SetTextColor(120, 120, 120)
			d.pdf.MultiCell(0, lineHeight, row.Label, "", "L", false)
		}
		d.pdf.SetTextColor(0, 0, 0)
		d.pdf.MultiCell(0, lineHeight, row.Value, "", "L", false)
		return
	}

	// Строку переносим целиком, иначе колонки разъедутся на разрыве страницы
	if d.pdf.GetY()+height > pageHeight-marginBottom {
		d.pdf.AddPage()
	}

	startY := d.pdf.GetY()

	d.pdf.SetXY(marginX, startY)
	d.pdf.SetTextColor(120, 120, 120)
	d.pdf.MultiCell(labelWidth, lineHeight, row.Label, "", "L", false)

	d.pdf.SetXY(marginX+labelWidth, startY)
	d.pdf.SetTextColor(0, 0, 0)
	d.pdf.MultiCell(valueWidth, lineHeight, row.Value, "", "L", false)

	d.pdf.SetXY(marginX, startY+height)
}

// attachments печатает опись вложений, а затем вклеивает каждое из них.
func (d *doc) attachments(files []Attachment, unsupportedTail string) {
	if len(files) == 0 {
		return
	}

	rows := make([]Row, 0, len(files))
	for i, file := range files {
		rows = append(rows, Row{
			Label: strconv.Itoa(i + 1),
			Value: file.Name + " (" + file.SizeLabel + ")",
		})
	}
	d.section(fmt.Sprintf("Прикреплённые файлы (%d)", len(files)), rows)

	for _, file := range files {
		d.appendFile(file, unsupportedTail)
	}
}

func (d *doc) appendFile(file Attachment, unsupportedTail string) {
	if file.Err != nil {
		d.placeholder(file, "Не удалось загрузить файл для встраивания.")
		return
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Name), "."))

	switch {
	case ext == "pdf":
		if err := d.appendPDF(file.Content); err != nil {
			d.placeholder(file, "Не удалось встроить PDF-файл (возможно, повреждён или защищён).")
		}
	case imageExtensions[ext]:
		d.appendImage(file)
	default:
		d.placeholder(file, "Файл этого типа нельзя встроить в PDF — "+unsupportedTail+".")
	}
}

// appendPDF вклеивает все страницы чужого PDF, сохраняя их размер.
func (d *doc) appendPDF(content []byte) (err error) {
	// gofpdi на битом или защищённом файле паникует вместо возврата ошибки
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("не удалось разобрать PDF: %v", recovered)
		}
	}()

	source := io.ReadSeeker(bytes.NewReader(content))
	// gofpdi различает источники по адресу указателя — держим его живым,
	// иначе следующее вложение может получить тот же адрес и чужие страницы.
	d.sources = append(d.sources, &source)

	first := d.imp.ImportPageFromStream(d.pdf, &source, 1, "/MediaBox")

	sizes := d.imp.GetPageSizes()
	if len(sizes) == 0 {
		return errors.New("в файле нет страниц")
	}

	d.placeTemplate(first, sizes[1])
	for page := 2; page <= len(sizes); page++ {
		d.placeTemplate(d.imp.ImportPageFromStream(d.pdf, &source, page, "/MediaBox"), sizes[page])
	}

	return nil
}

func (d *doc) placeTemplate(tpl int, size map[string]map[string]float64) {
	box := size["/MediaBox"]
	width, height := box["w"]*ptToMm, box["h"]*ptToMm
	if width <= 0 || height <= 0 {
		return
	}

	orientation := "P"
	if width > height {
		orientation = "L"
	}

	d.pdf.AddPageFormat(orientation, fpdf.SizeType{Wd: width, Ht: height})
	d.imp.UseImportedTemplate(d.pdf, tpl, 0, 0, width, height)
}

// appendImage кладёт картинку на отдельную страницу, вписывая её в поля.
func (d *doc) appendImage(file Attachment) {
	config, format, err := image.DecodeConfig(bytes.NewReader(file.Content))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		d.placeholder(file, "Не удалось обработать изображение.")
		return
	}

	d.pdf.AddPage()
	d.fileCaption(file)

	top := d.pdf.GetY() + 2
	pageWidth, pageHeight := d.pdf.GetPageSize()
	availableWidth := pageWidth - 2*marginX
	availableHeight := pageHeight - top - marginBottom

	scale := math.Min(availableWidth/float64(config.Width), availableHeight/float64(config.Height))
	if scale <= 0 {
		return
	}

	width := float64(config.Width) * scale
	height := float64(config.Height) * scale

	d.imageSeq++
	name := "attachment-" + strconv.Itoa(d.imageSeq)
	options := fpdf.ImageOptions{ImageType: format}

	d.pdf.RegisterImageOptionsReader(name, options, bytes.NewReader(file.Content))
	d.pdf.ImageOptions(name, marginX+(availableWidth-width)/2, top, width, height, false, options, 0, "")
}

func (d *doc) placeholder(file Attachment, reason string) {
	d.pdf.AddPage()
	d.fileCaption(file)

	d.pdf.SetFont(fontFamily, "", 10)
	d.pdf.SetTextColor(120, 120, 120)
	d.pdf.MultiCell(0, 6, reason, "", "L", false)
	d.pdf.SetTextColor(0, 0, 0)
}

func (d *doc) fileCaption(file Attachment) {
	d.pdf.SetFont(fontFamily, "B", 12)
	d.pdf.SetTextColor(0, 0, 0)
	d.pdf.MultiCell(0, 7, "Вложение: "+file.Name, "", "L", false)

	d.pdf.SetFont(fontFamily, "", 9)
	d.pdf.SetTextColor(120, 120, 120)
	d.pdf.MultiCell(0, 5, file.SizeLabel, "", "L", false)

	d.pdf.SetTextColor(0, 0, 0)
	d.pdf.Ln(1)
}

func nonEmpty(rows []Row) []Row {
	filtered := make([]Row, 0, len(rows))
	for _, row := range rows {
		if row.Value != "" {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
