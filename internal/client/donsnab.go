package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"go_external_api_document_flow/internal/config"
)

// maxAttachmentBytes ограничивает вложение, которое мы готовы загрузить в память
// ради вклейки в PDF. ponytail: если появятся файлы крупнее — резать на диск.
const maxAttachmentBytes = 64 << 20

// APIError — ошибка обращения к внешнему сервису don_snab.
// Status — HTTP-код upstream (0, если запрос вообще не дошёл).
type APIError struct {
	Status int
	Body   map[string]any
	Err    error
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("внешний сервис недоступен: %v", e.Err)
	}
	return fmt.Sprintf("код %d", e.Status)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// Message достаёт error.message из тела ответа upstream, если оно там есть.
func (e *APIError) Message() string {
	errObj, ok := e.Body["error"].(map[string]any)
	if !ok {
		return ""
	}
	msg, _ := errObj["message"].(string)
	return msg
}

// Violations собирает error.violations в строку «поле — сообщение; поле2 — сообщение2».
// Ключи сортируются: в PHP порядок задавал upstream, в Go порядок обхода map случайный.
func (e *APIError) Violations() string {
	errObj, ok := e.Body["error"].(map[string]any)
	if !ok {
		return ""
	}
	violations, ok := errObj["violations"].(map[string]any)
	if !ok || len(violations) == 0 {
		return ""
	}

	fields := make([]string, 0, len(violations))
	for field := range violations {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s — %v", field, violations[field]))
	}

	return strings.Join(parts, "; ")
}

type DonSnab struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewDonSnab(cfg config.DonSnabConfig) *DonSnab {
	return &DonSnab{
		baseURL: strings.TrimRight(cfg.APIURL, "/"),
		apiKey:  cfg.APIKey,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

// GetJSON выполняет GET и разбирает JSON-объект ответа. Успех — только 200, как в Symfony-клиентах.
func (c *DonSnab) GetJSON(ctx context.Context, path string, query url.Values) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	return decodeJSON(resp.Body)
}

// SendJSON выполняет POST/PATCH с JSON-телом.
// wantStatus > 0 — требуется ровно этот код; 0 — подойдёт любой 2xx.
func (c *DonSnab) SendJSON(ctx context.Context, method, path string, body any, wantStatus int) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, method, path, nil, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	okStatus := resp.StatusCode >= 200 && resp.StatusCode < 300
	if wantStatus > 0 {
		okStatus = resp.StatusCode == wantStatus
	}
	if !okStatus {
		return nil, c.apiError(resp)
	}

	// PATCH-ручки обращений и откликов ничего осмысленного не возвращают
	if resp.StatusCode == http.StatusNoContent {
		return map[string]any{}, nil
	}

	return decodeJSON(resp.Body)
}

// Fetch забирает бинарный ответ upstream в память — нужно для вклейки
// вложений в PDF, где файл не пробрасывается, а обрабатывается.
func (c *DonSnab) Fetch(ctx context.Context, path string, query url.Values) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	return io.ReadAll(io.LimitReader(resp.Body, maxAttachmentBytes))
}

// Stream проксирует бинарный ответ upstream (вложения, резюме) прямо в клиента.
// В отличие от Symfony-версии файл не загружается целиком в память.
func (c *DonSnab) Stream(ctx context.Context, w http.ResponseWriter, path string, query url.Values) error {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.apiError(resp)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	if length := resp.Header.Get("Content-Length"); length != "" {
		w.Header().Set("Content-Length", length)
	}

	// После WriteHeader ответ уже ушёл клиенту: обрыв на середине файла
	// нельзя превращать в JSON-ошибку — она просто дописалась бы в тело.
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("Обрыв проксирования файла", "path", path, "error", err)
	}

	return nil
}

func (c *DonSnab) do(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Response, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Status: 0, Err: err}
	}

	return resp, nil
}

// apiError читает тело ошибки (оно нужно для 422 с violations) и закрывает его.
func (c *DonSnab) apiError(resp *http.Response) *APIError {
	body, _ := decodeJSON(io.LimitReader(resp.Body, 1<<20))
	return &APIError{Status: resp.StatusCode, Body: body}
}

func decodeJSON(r io.Reader) (map[string]any, error) {
	var data map[string]any
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
