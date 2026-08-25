package voicemail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type APIError struct {
	Status int
	Body   map[string]any
	Err    error
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("vmapi недоступен: %v", e.Err)
	}
	return fmt.Sprintf("vmapi код %d", e.Status)
}

func (e *APIError) Message() string {
	if msg, ok := e.Body["error"].(string); ok {
		return msg
	}
	return ""
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		token:   cfg.Token,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	return c.GetJSON(ctx, "/api/v1/health", nil)
}

func (c *Client) GetJSON(ctx context.Context, path string, query url.Values) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	return decodeJSON(resp.Body)
}

func (c *Client) SendJSON(ctx context.Context, method, path string, body any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, method, path, nil, payload, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	return decodeJSON(resp.Body)
}

func (c *Client) Stream(ctx context.Context, w http.ResponseWriter, path string) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil, nil, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.apiError(resp)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	w.Header().Set("Content-Type", contentType)
	if length := resp.Header.Get("Content-Length"); length != "" {
		w.Header().Set("Content-Length", length)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)

	return nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, hasJSON bool) (*http.Response, error) {
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if hasJSON {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Status: 0, Err: err}
	}

	return resp, nil
}

func (c *Client) apiError(resp *http.Response) *APIError {
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
