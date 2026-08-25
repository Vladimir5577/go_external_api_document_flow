package dto

import "strconv"

var citizenAppealStatusLabels = map[string]string{
	"new":         "Новое",
	"in_progress": "В работе",
	"done":        "Обработано",
}

var citizenAppealTypeLabels = map[string]string{
	"individual_contract":        "Договор (физ. лицо)",
	"legal_contract":             "Договор (юр. лицо)",
	"receipt_data_correction":    "Корректировка квитанции",
	"recalculation":              "Перерасчёт",
	"waste_pickup_schedule":      "График вывоза мусора",
	"illegal_dump":               "Несанкционированная свалка",
	"bulky_waste":                "КГО",
	"container_site_improvement": "Контейнерная площадка",
	"other":                      "Другое",
}

// citizenAppealCityLabels использует только сборщик PDF: в JSON для SPA
// город уходит кодом, лейбл рисовался лишь в Twig и в выгрузке.
var citizenAppealCityLabels = map[string]string{
	"donetsk":      "Донецк",
	"makeyevka":    "Макеевка",
	"mariupol":     "Мариуполь",
	"gorlovka":     "Горловка",
	"enakievo":     "Енакиево",
	"shakhtersk":   "Шахтёрск",
	"torez":        "Торез",
	"amvrosievka":  "Амвросиевка",
	"yasinovataya": "Ясиноватая",
}

func CitizenAppealStatusLabel(v any) string {
	return label(citizenAppealStatusLabels, v)
}

func CitizenAppealTypeLabel(v any) string {
	return label(citizenAppealTypeLabels, v)
}

func CitizenAppealCityLabel(v any) string {
	return label(citizenAppealCityLabels, v)
}

func (p *Presenter) CitizenAppealList(raw map[string]any) map[string]any {
	out := p.list(raw, func(item map[string]any) map[string]any {
		return p.CitizenAppeal(item, false)
	})
	out["newAppealsCount"] = intOf(raw["newAppealsCount"])

	return out
}

// CitizenAppeal — порт SpaApi\ExternalApi\CitizenAppealController::present().
func (p *Presenter) CitizenAppeal(a map[string]any, full bool) map[string]any {
	out := map[string]any{
		"id":              intOf(a["id"]),
		"publicId":        a["publicId"],
		"fio":             a["fio"],
		"appealType":      a["appealType"],
		"appealTypeLabel": label(citizenAppealTypeLabels, a["appealType"]),
		"city":            a["city"],
		"status":          a["status"],
		"statusLabel":     label(citizenAppealStatusLabels, a["status"]),
		"createdAt":       p.atom(a["createdAt"]),
	}

	if !full {
		return out
	}

	out["phone"] = a["phone"]
	out["email"] = a["email"]
	out["address"] = a["address"]
	out["message"] = a["message"]
	out["replyTo"] = a["replyTo"]
	out["adminComment"] = a["adminComment"]
	out["updatedAt"] = p.atom(a["updatedAt"])
	out["files"] = files(a["files"], APIPrefix+"/citizen-appeals/files/")

	return out
}

// files — общая форма вложений для обращений и заявок на договор.
// url подменяем на собственный прокси: адрес upstream наружу не отдаём.
func files(raw any, urlPrefix string) []map[string]any {
	list, _ := raw.([]any)
	out := make([]map[string]any, 0, len(list))

	for _, item := range list {
		file, ok := item.(map[string]any)
		if !ok {
			continue
		}

		id := intOf(file["id"])
		out = append(out, map[string]any{
			"id":           id,
			"originalName": file["originalName"],
			"mimeType":     file["mimeType"],
			"fileSize":     intOf(file["fileSize"]),
			"sizeLabel":    sizeLabel(file["fileSize"]),
			"url":          urlPrefix + strconv.FormatInt(id, 10),
		})
	}

	return out
}
