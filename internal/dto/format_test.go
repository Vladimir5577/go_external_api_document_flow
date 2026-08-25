package dto

import (
	"encoding/json"
	"testing"
	"time"
)

// Форма ответа — контракт с фронтом analytics_platform. Тест сверяет её
// с тем, что отдавали контроллеры SpaApi\ExternalApi\** в монолите.

func TestSizeLabelMatchesPhpFormatting(t *testing.T) {
	cases := map[float64]string{
		0:         "0 КБ",
		1024:      "1 КБ",
		1536:      "1.5 КБ",
		1024 * 12: "12 КБ",
		1048576:   "1 МБ",
		1572864:   "1.5 МБ",
	}

	for bytes, want := range cases {
		if got := sizeLabel(bytes); got != want {
			t.Errorf("sizeLabel(%v) = %q, ожидалось %q", bytes, got, want)
		}
	}
}

func TestAtomKeepsRfc3339AndFillsOffset(t *testing.T) {
	p := New(time.FixedZone("MSK", 3*60*60))

	if got := p.atom("2026-08-12T10:00:00+03:00"); got != "2026-08-12T10:00:00+03:00" {
		t.Errorf("RFC3339 должен проходить без изменений, получено %v", got)
	}
	if got := p.atom("2026-08-12 10:00:00"); got != "2026-08-12T10:00:00+03:00" {
		t.Errorf("дата без смещения должна получить таймзону приложения, получено %v", got)
	}
}

func TestCitizenAppealFullShape(t *testing.T) {
	p := New(time.UTC)

	got := p.CitizenAppeal(map[string]any{
		"id":         float64(7),
		"publicId":   "OG-7",
		"fio":        "Иванов И.И.",
		"appealType": "bulky_waste",
		"city":       "donetsk",
		"status":     "in_progress",
		"createdAt":  "2026-08-12T10:00:00+03:00",
		"updatedAt":  "2026-08-12T11:00:00+03:00",
		"files": []any{map[string]any{
			"id":           float64(3),
			"originalName": "скан.pdf",
			"mimeType":     "application/pdf",
			"fileSize":     float64(1536),
			"url":          "https://upstream.example/internal/3",
		}},
	}, true)

	if got["appealTypeLabel"] != "КГО" || got["statusLabel"] != "В работе" {
		t.Errorf("лейблы не совпали: %v / %v", got["appealTypeLabel"], got["statusLabel"])
	}

	files := got["files"].([]map[string]any)
	if len(files) != 1 {
		t.Fatalf("ожидался один файл, получено %d", len(files))
	}
	// Адрес upstream наружу уходить не должен — только наш прокси-путь.
	if files[0]["url"] != "/spa/api/external-api/citizen-appeals/files/3" {
		t.Errorf("url файла = %v", files[0]["url"])
	}
	if files[0]["sizeLabel"] != "1.5 КБ" {
		t.Errorf("sizeLabel = %v", files[0]["sizeLabel"])
	}

	// id в JSON должен остаться целым числом, а не 7e+00
	encoded, _ := json.Marshal(got)
	if !contains(string(encoded), `"id":7`) {
		t.Errorf("id сериализован не как целое: %s", encoded)
	}
}

func TestContractApplicationFallsBackToConsumerBlock(t *testing.T) {
	p := New(time.UTC)

	got := p.ContractApplication(map[string]any{
		"id":       float64(1),
		"publicId": "ZD-1",
		"status":   "new",
		"consumer": map[string]any{
			"type":         "ip",
			"name":         "ИП Петров",
			"organization": "Петров и Ко",
		},
	}, false)

	if got["consumerType"] != "ip" || got["consumerTypeLabel"] != "ИП" {
		t.Errorf("тип потребителя не подтянулся из блока consumer: %v", got)
	}
	if got["consumerName"] != "ИП Петров" || got["organization"] != "Петров и Ко" {
		t.Errorf("имя/организация не подтянулись из блока consumer: %v", got)
	}
}

func TestListWrapsItemsAndPagination(t *testing.T) {
	p := New(time.UTC)

	got := p.VacancyApplicationList(map[string]any{
		"data": []any{
			map[string]any{"id": float64(1), "status": "invited", "createdAt": "2026-08-12T10:00:00+03:00"},
		},
		"pagination": map[string]any{"page": float64(2), "limit": float64(20), "total": float64(21), "pages": float64(2)},
	})

	items := got["items"].([]map[string]any)
	if len(items) != 1 || items[0]["statusLabel"] != "Приглашён" {
		t.Errorf("элементы списка собраны неверно: %v", items)
	}

	pagination := got["pagination"].(map[string]any)
	if pagination["page"] != int64(2) || pagination["total"] != int64(21) {
		t.Errorf("пагинация собрана неверно: %v", pagination)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
