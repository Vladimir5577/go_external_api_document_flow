// Package dto повторяет форму JSON-ответов, которую отдавали контроллеры
// symfony_documents_flow/src/Controller/SpaApi/ExternalApi/**. Фронт (analytics_platform)
// уже завязан на неё, поэтому это контракт, а не свободное творчество.
package dto

import (
	"math"
	"strconv"
	"time"
)

// APIPrefix — общий префикс всех маршрутов сервиса. Один префикс на сервис
// позволяет разрезать трафик в nginx одним правилом, как сделано с канбаном,
// и не ловить чужие ручки, добавленные в монолите под теми же именами.
const APIPrefix = "/spa/api/external-api"

type Presenter struct {
	loc *time.Location
}

func New(loc *time.Location) *Presenter {
	return &Presenter{loc: loc}
}

// list — общая для всех четырёх модулей обёртка списка:
// upstream отдаёт {data: [...], pagination: {...}}, наружу уходит {items: [...], pagination: {...}}.
func (p *Presenter) list(raw map[string]any, present func(map[string]any) map[string]any) map[string]any {
	rawItems, _ := raw["data"].([]any)
	items := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		if obj, ok := item.(map[string]any); ok {
			items = append(items, present(obj))
		}
	}

	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":  intOf(value(raw, "pagination", "page")),
			"limit": intOf(value(raw, "pagination", "limit")),
			"total": intOf(value(raw, "pagination", "total")),
			"pages": intOf(value(raw, "pagination", "pages")),
		},
	}
}

// Data вытаскивает объект из конверта {data: {...}} ответа upstream.
func Data(raw map[string]any) map[string]any {
	obj, _ := raw["data"].(map[string]any)
	return obj
}

// value достаёт значение по вложенному пути; nil, если по дороге чего-то нет.
func value(m map[string]any, path ...string) any {
	var current any = m
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[key]
		if !ok {
			return nil
		}
	}
	return current
}

// firstOf возвращает первое не-nil значение — порт цепочек `?? ... ?? null` из PHP-DTO.
func firstOf(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func intOf(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case string:
		parsed, _ := strconv.ParseInt(n, 10, 64)
		return parsed
	}
	return 0
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

// label — порт `match($code) { ... default => $code }`: неизвестный код отдаём как есть.
func label(dict map[string]string, code any) string {
	raw := str(code)
	if translated, ok := dict[raw]; ok {
		return translated
	}
	return raw
}

// sizeLabel — порт FileDto::getFileSizeFormatted().
func sizeLabel(v any) string {
	kb := float64(intOf(v)) / 1024
	if kb < 1024 {
		return round1(kb) + " КБ"
	}
	return round1(kb/1024) + " МБ"
}

// round1 повторяет `round($x, 1)` с последующим приведением к строке:
// целые значения PHP печатает без дробной части.
func round1(x float64) string {
	return strconv.FormatFloat(math.Round(x*10)/10, 'f', -1, 64)
}

// SizeLabel — публичная обёртка для сборщика PDF.
func SizeLabel(v any) string {
	return sizeLabel(v)
}

// atom — порт `new \DateTimeImmutable($v)->format(\DateTimeInterface::ATOM)`.
// Значения, уже пришедшие в RFC3339, отдаём без изменений; строки без смещения
// трактуем в таймзоне приложения, как это делал PHP по date.timezone.
func (p *Presenter) atom(v any) any {
	raw := str(v)
	if raw == "" {
		return v
	}

	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw
	}

	if parsed, ok := p.parseTime(raw); ok {
		return parsed.Format(time.RFC3339)
	}

	return raw
}

// DateTime — порт `->format('d.m.Y H:i')`, в этом виде даты попадают в PDF.
func (p *Presenter) DateTime(v any) string {
	raw := str(v)
	if raw == "" {
		return ""
	}

	if parsed, ok := p.parseTime(raw); ok {
		return parsed.Format("02.01.2006 15:04")
	}

	return raw
}

func (p *Presenter) parseTime(raw string) (time.Time, bool) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, true
	}

	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, raw, p.loc); err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}
