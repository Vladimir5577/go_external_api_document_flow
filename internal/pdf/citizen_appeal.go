package pdf

import (
	"context"

	"go_external_api_document_flow/internal/dto"
)

// CitizenAppeal — порт CitizenAppealPdfService::generate().
// На вход идёт сырой объект из don_snab, а не презентация для SPA:
// выгрузке нужны поля, которых в JSON для фронта нет (город лейблом, адрес).
func (r *Renderer) CitizenAppeal(ctx context.Context, appeal map[string]any, files []Attachment) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	heading := "Обращение " + text(appeal["publicId"])

	d := r.newDoc(heading)
	d.title(heading)

	d.rows([]Row{
		{"Статус", dto.CitizenAppealStatusLabel(appeal["status"])},
		{"Дата подачи", r.presenter.DateTime(appeal["createdAt"])},
	})

	d.section("Данные заявителя", []Row{
		{"ФИО", text(appeal["fio"])},
		{"Телефон", text(appeal["phone"])},
		{"Email", text(appeal["email"])},
	})

	d.section("Обращение", []Row{
		{"Тип обращения", dto.CitizenAppealTypeLabel(appeal["appealType"])},
		{"Город", dto.CitizenAppealCityLabel(appeal["city"])},
		{"Адрес", text(appeal["address"])},
	})

	d.section("Текст обращения", []Row{{"", text(appeal["message"])}})
	d.section("Комментарий администратора", []Row{{"", text(appeal["adminComment"])}})

	d.attachments(files, "он приложен к обращению отдельно")

	return d.output()
}
