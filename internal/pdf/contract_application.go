package pdf

import (
	"context"

	"go_external_api_document_flow/internal/dto"
)

// Словари, которые в монолите жили прямо в PDF-сервисе: в JSON для SPA
// эти поля уходят кодами, лейблы нужны только в выгрузке.
var (
	signerTypeLabels = map[string]string{
		"director":       "Руководитель",
		"representative": "Представитель по доверенности",
	}

	deliveryMethodLabels = map[string]string{
		"edo":     "ЭДО",
		"mail":    "Почта",
		"courier": "Курьер",
		"pickup":  "Самовывоз",
	}

	objectCategoryLabels = map[string]string{
		"commercial":  "Коммерческая",
		"residential": "Жилая",
		"industrial":  "Промышленная",
	}

	ownershipLabels = map[string]string{
		"own":  "Собственность",
		"rent": "Аренда",
	}

	containerOwnershipLabels = map[string]string{
		"operator": "Оператора",
		"own":      "Собственные",
	}

	containerMaterialLabels = map[string]string{
		"metal":   "Металл",
		"plastic": "Пластик",
	}

	containerScheduleLabels = map[string]string{
		"daily":      "Ежедневно",
		"twice_week": "2 раза в неделю",
		"once_week":  "1 раз в неделю",
	}
)

// ContractApplication — порт ContractApplicationPdfService::generate().
func (r *Renderer) ContractApplication(ctx context.Context, application map[string]any, files []Attachment) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	heading := "Заявка " + text(application["publicId"])

	d := r.newDoc(heading)
	d.title(heading)

	d.rows([]Row{
		{"Номер", text(application["publicId"])},
		{"Статус", dto.ContractApplicationStatusLabel(application["status"])},
		{"Дата подачи", r.presenter.DateTime(application["createdAt"])},
	})

	r.consumerSection(d, application)

	d.section("Комментарий администратора", []Row{{"", text(application["adminComment"])}})

	requisites := block(application, "requisites")
	d.section("Реквизиты", []Row{
		{"Полное наименование", text(requisites["organizationName"])},
		{"Юридический адрес", text(requisites["legalAddress"])},
		{"Фактический адрес", text(requisites["actualAddress"])},
		{"ИНН", text(requisites["inn"])},
		{"КПП", text(requisites["kpp"])},
		{"ОГРН", text(requisites["ogrn"])},
		{"Руководитель", joinNonEmpty(", ", text(requisites["director"]), text(requisites["directorPosition"]))},
		{"Представитель", text(requisites["representativeName"])},
		{"Телефон организации", text(requisites["orgPhone"])},
		{"Email организации", text(requisites["orgEmail"])},
		{"Расчётный счёт", text(requisites["bankAccount"])},
		{"Банк", text(requisites["bankName"])},
		{"БИК", text(requisites["bankBik"])},
		{"Корр. счёт", text(requisites["bankCorr"])},
		{"ФИО", text(requisites["personName"])},
		{"Адрес", text(requisites["personAddress"])},
		{"Паспорт серия", text(requisites["passportSeries"])},
		{"Паспорт выдан", joinNonEmpty(" ", text(requisites["passportIssuedBy"]), text(requisites["passportIssuedDate"]))},
	})

	signer := block(application, "signer")
	edo := text(signer["edo"]) == "yes"
	d.section("Подписант и доставка", []Row{
		{"Подписант", label(signerTypeLabels, signer["signerType"])},
		{"ФИО подписанта", text(signer["signerName"])},
		{"Должность", text(signer["signerPosition"])},
		{"Телефон подписанта", text(signer["signerPhone"])},
		{"Email подписанта", text(signer["signerEmail"])},
		{"Документ", text(signer["signerDocument"])},
		{"Способ доставки", label(deliveryMethodLabels, signer["deliveryMethod"])},
		{"ЭДО оператор", dash(edo, text(signer["edoOperator"]))},
		{"ЭДО ID", dash(edo, text(signer["edoId"]))},
	})

	waste := block(application, "waste")
	d.section("Сведения об отходах", []Row{
		{"Категория объекта", label(objectCategoryLabels, waste["objectCategory"])},
		{"Вид отходов", text(waste["wasteName"])},
		{"Код ФККО", text(waste["wasteCode"])},
		{"Объём", withUnit(waste["calcAmount"], wasteUnit(waste))},
		{"Паспорт отходов", choice(waste, "wastePassport", "yes", "Есть", "Нет")},
	})

	site := block(application, "site")
	d.section("Объект", []Row{
		{"Название", text(site["siteName"])},
		{"Адрес", text(site["siteAddress"])},
		{"Вид деятельности", text(site["siteActivity"])},
		{"Площадь", withUnit(site["siteArea"], "м²")},
		{"Численность сотрудников", text(site["siteStaffCount"])},
		{"Назначение", choice(site, "sitePurpose", "residential", "Жилое", "Нежилое")},
		{"Право пользования", label(ownershipLabels, site["ownership"])},
		{"Документ на собственность", text(site["ownershipDocDetails"])},
		{"Документ на аренду", text(site["rentDocDetails"])},
	})

	containers := block(application, "containers")
	d.section("Контейнеры и вывоз", []Row{
		{"Контейнеры", label(containerOwnershipLabels, containers["containerOwnership"])},
		{"Материал", label(containerMaterialLabels, containers["containerMaterial"])},
		{"Тип контейнера", text(containers["containerKind"])},
		{"Объём контейнера", containerVolume(containers)},
		{"Количество", withUnit(containers["containerCount"], "шт.")},
		{"График вывоза", label(containerScheduleLabels, containers["containerSchedule"])},
		{"Место накопления", text(containers["accumulationAddress"])},
	})

	extra := block(application, "extra")
	d.section("Дополнительная информация", []Row{
		{"Контактное лицо на объекте", text(extra["siteContactName"])},
		{"Телефон контактного лица", text(extra["siteContactPhone"])},
		{"Условия доступа", text(extra["accessConditions"])},
		{"Сообщение", text(extra["message"])},
	})

	d.attachments(files, "он приложен к заявке отдельно")

	return d.output()
}

// consumerSection повторяет цепочки `?? consumer[...]` из ContractApplicationDto.
func (r *Renderer) consumerSection(d *doc, application map[string]any) {
	consumer := block(application, "consumer")

	d.section("Данные потребителя", []Row{
		{"Тип", dto.ContractConsumerTypeLabel(first(text(application["consumerType"]), text(consumer["type"])))},
		{"Наименование", first(text(application["consumerName"]), text(consumer["name"]))},
		{"Организация", first(text(application["organization"]), text(consumer["organization"]))},
		{"Телефон", first(text(application["primaryPhone"]), text(consumer["primaryPhone"]))},
		{"Email", first(text(application["primaryEmail"]), text(consumer["primaryEmail"]))},
	})
}

// wasteUnit: m3 показываем значком, остальные единицы — как пришли.
func wasteUnit(waste map[string]any) string {
	if text(waste["calcUnit"]) == "m3" {
		return "м³"
	}
	return text(waste["calcUnit"])
}

func containerVolume(containers map[string]any) string {
	volume := withUnit(containers["containerVolume"], "м³")
	if volume == "" {
		return ""
	}
	if other := text(containers["containerVolumeOther"]); other != "" {
		volume += " (" + other + ")"
	}
	return volume
}

// dash — поля ЭДО показываем только при edo=yes, пустое значение заменяя прочерком.
func dash(show bool, value string) string {
	if !show {
		return ""
	}
	if value == "" {
		return "—"
	}
	return value
}
