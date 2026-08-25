package dto

import "strconv"

var vacancyApplicationStatusLabels = map[string]string{
	"new":      "Новый",
	"viewed":   "Просмотрен",
	"invited":  "Приглашён",
	"rejected": "Отказ",
	"archived": "Архив",
}

func (p *Presenter) VacancyApplicationList(raw map[string]any) map[string]any {
	return p.list(raw, func(item map[string]any) map[string]any {
		return p.VacancyApplication(item, false)
	})
}

// VacancyApplication — порт SpaApi\ExternalApi\Hr\VacancyApplicationController::present().
func (p *Presenter) VacancyApplication(a map[string]any, full bool) map[string]any {
	id := intOf(a["id"])

	out := map[string]any{
		"id":                   id,
		"vacancyId":            intOf(a["vacancyId"]),
		"vacancyTitleSnapshot": a["vacancyTitleSnapshot"],
		"fio":                  a["fio"],
		"status":               a["status"],
		"statusLabel":          label(vacancyApplicationStatusLabels, a["status"]),
		"createdAt":            p.atom(a["createdAt"]),
	}

	if !full {
		return out
	}

	out["vacancySlug"] = a["vacancySlug"]
	out["phone"] = a["phone"]
	out["email"] = a["email"]
	out["coverLetter"] = a["coverLetter"]
	out["adminComment"] = a["adminComment"]
	out["updatedAt"] = p.atom(a["updatedAt"])
	out["resume"] = resume(a["resume"], id)

	return out
}

func resume(raw any, applicationID int64) any {
	file, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	return map[string]any{
		"originalName": file["originalName"],
		"mimeType":     file["mimeType"],
		"size":         intOf(file["size"]),
		"sizeLabel":    sizeLabel(file["size"]),
		"url":          APIPrefix + "/vacancy-applications/" + strconv.FormatInt(applicationID, 10) + "/resume",
	}
}
