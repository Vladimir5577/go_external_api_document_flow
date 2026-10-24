package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ListLimit matches the hardcoded page size from the Symfony SPA controllers.
const ListLimit = "20"

var ErrInvalidJSON = errors.New("Invalid JSON")

// maxBodyBytes — потолок JSON-тела PATCH/POST: формы шлюза занимают килобайты.
const maxBodyBytes = 1 << 20

// PageParam repeats $request->query->getInt('page', 1):
// absent key means 1, present non-number means 0.
func PageParam(r *http.Request) string {
	raw := r.URL.Query().Get("page")
	if raw == "" {
		return "1"
	}
	if _, err := strconv.Atoi(raw); err != nil {
		return "0"
	}
	return raw
}

// CopyFilters copies non-empty query params into the upstream request.
func CopyFilters(dst url.Values, r *http.Request, keys ...string) {
	query := r.URL.Query()
	for _, key := range keys {
		if v := query.Get(key); v != "" {
			dst.Set(key, v)
		}
	}
}

func QueryOr(r *http.Request, key, fallback string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	return fallback
}

// DownloadParam forwards truthy download values as download=1.
func DownloadParam(r *http.Request) url.Values {
	raw := r.URL.Query().Get("download")
	if raw == "" || raw == "0" || raw == "false" {
		return nil
	}
	return url.Values{"download": []string{"1"}}
}

func IDParam(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func DecodeBody(r *http.Request) (map[string]any, error) {
	var body map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes)).Decode(&body); err != nil || body == nil {
		return nil, ErrInvalidJSON
	}
	return body, nil
}

// PickPresent пропускает отсутствующие поля; null у adminComment очищает комментарий.
func PickPresent(body map[string]any, keys ...string) map[string]any {
	payload := make(map[string]any, len(keys))
	for _, key := range keys {
		v, ok := body[key]
		if ok && key == "adminComment" && v == nil {
			v = ""
		}
		if ok && v != nil {
			payload[key] = v
		}
	}
	return payload
}

// Truthy repeats PHP bool coercion for JSON values used by the legacy controllers.
func Truthy(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		return value != "" && value != "0"
	case nil:
		return false
	}
	return true
}
