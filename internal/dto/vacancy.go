package dto

func (p *Presenter) VacancyList(raw map[string]any) map[string]any {
	return p.list(raw, func(item map[string]any) map[string]any {
		return p.Vacancy(item, false)
	})
}

// Vacancy — порт SpaApi\ExternalApi\Hr\VacancyController::present().
// Справочники city/employmentType/schedule/experience upstream отдаёт уже
// с готовыми label — переводим только форму, не значения.
func (p *Presenter) Vacancy(v map[string]any, full bool) map[string]any {
	out := map[string]any{
		"id":          intOf(v["id"]),
		"slug":        v["slug"],
		"title":       v["title"],
		"salary":      v["salary"],
		"city":        valueLabel(v["city"]),
		"isPublished": boolOf(v["isPublished"]),
		"sortOrder":   intOf(v["sortOrder"]),
		"createdAt":   p.atom(v["createdAt"]),
	}

	if !full {
		return out
	}

	out["employmentType"] = valueLabel(v["employmentType"])
	out["schedule"] = valueLabel(v["schedule"])
	out["experience"] = valueLabel(v["experience"])
	out["shortDescription"] = v["shortDescription"]
	out["bodyBlocks"] = bodyBlocks(v["bodyBlocks"])
	out["contactEmail"] = v["contactEmail"]
	out["contactPhone"] = v["contactPhone"]
	out["updatedAt"] = p.atom(v["updatedAt"])

	return out
}

// valueLabel нормализует справочное поле до {value, label} — лишние ключи upstream наружу не идут.
func valueLabel(raw any) map[string]any {
	obj, _ := raw.(map[string]any)
	return map[string]any{
		"value": str(obj["value"]),
		"label": str(obj["label"]),
	}
}

// bodyBlocks отдаём как есть, но nil превращаем в пустой массив: PHP-DTO делал `?? []`.
func bodyBlocks(raw any) any {
	if blocks, ok := raw.([]any); ok {
		return blocks
	}
	return []any{}
}
