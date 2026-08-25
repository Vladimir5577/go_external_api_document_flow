package dto

var contractApplicationStatusLabels = map[string]string{
	"new":           "Новая",
	"in_review":     "На проверке",
	"contract_sent": "Договор отправлен",
	"signed":        "Подписан",
	"rejected":      "Отклонена",
}

var contractConsumerTypeLabels = map[string]string{
	"legal":  "Юридическое лицо",
	"ip":     "ИП",
	"person": "Физическое лицо",
}

func ContractApplicationStatusLabel(v any) string {
	return label(contractApplicationStatusLabels, v)
}

func ContractConsumerTypeLabel(v any) string {
	return label(contractConsumerTypeLabels, v)
}

func (p *Presenter) ContractApplicationList(raw map[string]any) map[string]any {
	return p.list(raw, func(item map[string]any) map[string]any {
		return p.ContractApplication(item, false)
	})
}

// ContractApplication — порт SpaApi\ExternalApi\ContractApplicationController::present().
// Плоские поля потребителя upstream отдаёт либо верхним уровнем, либо внутри consumer —
// цепочки firstOf повторяют `??` из ContractApplicationDto::fromArray().
func (p *Presenter) ContractApplication(a map[string]any, full bool) map[string]any {
	consumerType := firstOf(a["consumerType"], value(a, "consumer", "type"))

	out := map[string]any{
		"id":                intOf(a["id"]),
		"publicId":          a["publicId"],
		"consumerType":      str(consumerType),
		"consumerTypeLabel": label(contractConsumerTypeLabels, consumerType),
		"consumerName":      str(firstOf(a["consumerName"], value(a, "consumer", "name"))),
		"organization":      firstOf(a["organization"], value(a, "consumer", "organization")),
		"status":            a["status"],
		"statusLabel":       label(contractApplicationStatusLabels, a["status"]),
		"createdAt":         p.atom(a["createdAt"]),
	}

	if !full {
		return out
	}

	out["primaryPhone"] = firstOf(a["primaryPhone"], value(a, "consumer", "primaryPhone"))
	out["primaryEmail"] = firstOf(a["primaryEmail"], value(a, "consumer", "primaryEmail"))
	out["adminComment"] = a["adminComment"]
	out["updatedAt"] = p.atom(a["updatedAt"])
	out["consumer"] = a["consumer"]
	out["requisites"] = a["requisites"]
	out["signer"] = a["signer"]
	out["waste"] = a["waste"]
	out["site"] = a["site"]
	out["containers"] = a["containers"]
	out["extra"] = a["extra"]
	out["files"] = files(a["files"], APIPrefix+"/contract-applications/files/")

	return out
}
