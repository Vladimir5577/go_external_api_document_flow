package pdf

import (
	"fmt"
	"strconv"
	"strings"
)

// text приводит значение из JSON upstream к строке так, как это делала
// конкатенация в PHP: числа без хвостового нуля, отсутствующее значение — пусто.
func text(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		if value {
			return "Да"
		}
		return "Нет"
	}
	return fmt.Sprint(v)
}

// block достаёт вложенный объект (requisites, signer, waste и т.п.).
// Возвращает nil, если его нет: чтение из nil-map в Go безопасно.
func block(m map[string]any, key string) map[string]any {
	nested, _ := m[key].(map[string]any)
	return nested
}

// first — порт цепочки `?? ... ?? null`: первое непустое значение.
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// label — порт `match($v) { ... null, ” => null, default => $v }`.
func label(dict map[string]string, v any) string {
	raw := text(v)
	if raw == "" {
		return ""
	}
	if translated, ok := dict[raw]; ok {
		return translated
	}
	return raw
}

// choice — порт `isset($x) ? ($x === 'match' ? a : b) : null`.
func choice(m map[string]any, key, match, whenMatch, otherwise string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if text(v) == match {
		return whenMatch
	}
	return otherwise
}

// withUnit добавляет единицу измерения, но только к непустому значению.
func withUnit(v any, unit string) string {
	raw := text(v)
	if raw == "" || unit == "" {
		return raw
	}
	return raw + " " + unit
}

// joinNonEmpty — порт одноимённого метода PHP-сервиса.
func joinNonEmpty(glue string, parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, glue)
}
