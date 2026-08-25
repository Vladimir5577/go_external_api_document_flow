package pdf

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go_external_api_document_flow/internal/dto"

	"github.com/go-pdf/fpdf"
	"github.com/go-pdf/fpdf/contrib/gofpdi"
)

func newTestRenderer(t *testing.T) *Renderer {
	t.Helper()

	regular := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	bold := "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
	for _, path := range []string{regular, bold} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("нет системного шрифта %s — установите fonts-dejavu-core", path)
		}
	}

	renderer, err := NewRenderer(regular, bold, dto.New(time.UTC))
	if err != nil {
		t.Fatalf("не удалось создать рендерер: %v", err)
	}

	return renderer
}

// samplePDF собирает многостраничный PDF, который потом вклеивается как вложение.
func samplePDF(t *testing.T, pages int) []byte {
	t.Helper()

	p := fpdf.New("P", "mm", "A4", "")
	for i := 0; i < pages; i++ {
		p.AddPage()
		p.SetFont("Helvetica", "", 12)
		p.Cell(40, 10, "attachment page")
	}

	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		t.Fatalf("не удалось собрать тестовый PDF: %v", err)
	}

	return buf.Bytes()
}

func samplePNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	for x := 0; x < 120; x++ {
		for y := 0; y < 60; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("не удалось собрать тестовый PNG: %v", err)
	}

	return buf.Bytes()
}

// pageCount читает получившийся PDF тем же импортером, которым вклеиваются вложения.
func pageCount(t *testing.T, content []byte) int {
	t.Helper()

	probe := fpdf.New("P", "mm", "A4", "")
	probe.AddPage()

	importer := gofpdi.NewImporter()
	source := io.ReadSeeker(bytes.NewReader(content))
	importer.ImportPageFromStream(probe, &source, 1, "/MediaBox")

	return len(importer.GetPageSizes())
}

func TestContractApplicationPdf(t *testing.T) {
	renderer := newTestRenderer(t)

	application := map[string]any{
		"publicId":     "ZD-42",
		"status":       "in_review",
		"createdAt":    "2026-08-12T10:30:00+03:00",
		"adminComment": "Проверить реквизиты",
		"consumer": map[string]any{
			"type":         "ip",
			"name":         "ИП Петров",
			"organization": "Петров и Ко",
			"primaryPhone": "+7 999 000-00-00",
		},
		"requisites": map[string]any{
			"organizationName": "ООО «Ромашка»",
			"inn":              "1234567890",
			"director":         "Сидоров С.С.",
			"directorPosition": "директор",
		},
		"signer": map[string]any{
			"signerType":     "director",
			"deliveryMethod": "courier",
			"edo":            "yes",
			"edoOperator":    "СБИС",
		},
		"waste": map[string]any{
			"objectCategory": "commercial",
			"calcAmount":     float64(12),
			"calcUnit":       "m3",
			"wastePassport":  "yes",
		},
		"site": map[string]any{
			"siteName":    "Склад №3",
			"siteArea":    float64(250),
			"sitePurpose": "residential",
			"ownership":   "rent",
		},
		"containers": map[string]any{
			"containerOwnership":   "own",
			"containerMaterial":    "metal",
			"containerVolume":      float64(1),
			"containerVolumeOther": "евроконтейнер",
			"containerCount":       float64(3),
			"containerSchedule":    "twice_week",
		},
		"extra": map[string]any{
			"message": strings.Repeat("Очень длинное сообщение заявителя. ", 200),
		},
	}

	files := []Attachment{
		{Name: "скан.pdf", SizeLabel: "1.5 КБ", Content: samplePDF(t, 2)},
		{Name: "фото.png", SizeLabel: "3 КБ", Content: samplePNG(t)},
		{Name: "битый.pdf", SizeLabel: "1 КБ", Content: []byte("не pdf вовсе")},
		{Name: "договор.docx", SizeLabel: "10 КБ", Content: []byte("docx")},
		{Name: "недоступный.pdf", SizeLabel: "2 КБ", Err: io.ErrUnexpectedEOF},
	}

	content, err := renderer.ContractApplication(context.Background(), application, files)
	if err != nil {
		t.Fatalf("генерация упала: %v", err)
	}

	if !bytes.HasPrefix(content, []byte("%PDF")) {
		t.Fatalf("на выходе не PDF: %q", content[:min(16, len(content))])
	}

	// 1+ страница описи, 2 вклеенные страницы, картинка,
	// и по заглушке на битый, неподдерживаемый и недоступный файлы
	if pages := pageCount(t, content); pages < 8 {
		t.Errorf("страниц в PDF = %d, ожидалось не меньше 8", pages)
	}

	dump(t, "contract-application.pdf", content)
}

// dump кладёт получившийся PDF на диск, если задан PDF_DUMP — чтобы посмотреть
// глазами: `PDF_DUMP=/tmp go test ./internal/pdf/`.
func dump(t *testing.T, name string, content []byte) {
	t.Helper()

	dir := os.Getenv("PDF_DUMP")
	if dir == "" {
		return
	}

	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		t.Errorf("не удалось сохранить PDF: %v", err)
	}
}

func TestCitizenAppealPdfWithoutAttachments(t *testing.T) {
	renderer := newTestRenderer(t)

	content, err := renderer.CitizenAppeal(context.Background(), map[string]any{
		"publicId":   "OG-7",
		"status":     "done",
		"createdAt":  "2026-08-12T10:00:00+03:00",
		"fio":        "Иванов Иван Иванович",
		"phone":      "+7 999 111-22-33",
		"appealType": "bulky_waste",
		"city":       "donetsk",
		"address":    "ул. Артёма, 1",
		"message":    "Не вывозят КГО две недели",
	}, nil)
	if err != nil {
		t.Fatalf("генерация упала: %v", err)
	}

	if pages := pageCount(t, content); pages != 1 {
		t.Errorf("страниц = %d, для обращения без вложений ожидалась одна", pages)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
