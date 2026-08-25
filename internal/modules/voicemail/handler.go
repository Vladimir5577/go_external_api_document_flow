package voicemail

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go_external_api_document_flow/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	client    *Client
	presenter *Presenter
}

func NewHandler(client *Client, presenter *Presenter) *Handler {
	return &Handler{client: client, presenter: presenter}
}

func (h *Handler) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := h.client.Health(r.Context())
		if err != nil {
			writeVMAPIError(w, err, "Сервис голосовой почты недоступен")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.Health(raw))
	}
}

func (h *Handler) Mailboxes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := h.client.GetJSON(r.Context(), "/api/v1/mailboxes", nil)
		if err != nil {
			writeVMAPIError(w, err, "Сервис голосовой почты недоступен")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.Mailboxes(raw))
	}
}

func (h *Handler) Messages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mailbox, ok := mailboxParam(r)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "Некорректный номер голосового ящика")
			return
		}

		query := url.Values{}
		httpx.CopyFilters(query, r, "audio", "limit")

		raw, err := h.client.GetJSON(r.Context(), "/api/v1/mailboxes/"+mailbox+"/messages", query)
		if err != nil {
			writeVMAPIError(w, err, "Не удалось получить голосовые обращения")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.Messages(raw))
	}
}

func (h *Handler) Ack() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mailbox, ok := mailboxParam(r)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "Некорректный номер голосового ящика")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		ids, ok := body["ids"].([]any)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "ids is required")
			return
		}

		payload := map[string]any{"ids": ids}
		raw, err := h.client.SendJSON(r.Context(), http.MethodPost, "/api/v1/mailboxes/"+mailbox+"/ack", payload)
		if err != nil {
			writeVMAPIError(w, err, "Не удалось подтвердить голосовые обращения")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.Ack(raw))
	}
}

func (h *Handler) Audio() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mailbox, ok := mailboxParam(r)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "Некорректный номер голосового ящика")
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			httpx.WriteError(w, http.StatusNotFound, "Запись не найдена")
			return
		}

		httpx.NoStore(w)
		path := "/api/v1/mailboxes/" + mailbox + "/messages/" + url.PathEscape(id) + "/audio"
		if err := h.client.Stream(r.Context(), w, path); err != nil {
			writeVMAPIError(w, err, "Не удалось получить запись")
			return
		}
	}
}

func mailboxParam(r *http.Request) (string, bool) {
	mailbox := chi.URLParam(r, "mailbox")
	if mailbox == "" {
		return "", false
	}
	return url.PathEscape(mailbox), true
}

func writeVMAPIError(w http.ResponseWriter, err error, fallback string) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		httpx.WriteError(w, http.StatusBadGateway, fallback+" (нет ответа)")
		return
	}

	message := apiErr.Message()
	if message == "" {
		message = fallback
	}

	switch apiErr.Status {
	case 0:
		httpx.WriteError(w, http.StatusBadGateway, fallback+" (нет ответа)")
	case http.StatusBadRequest:
		httpx.WriteError(w, http.StatusBadRequest, message)
	case http.StatusUnauthorized, http.StatusForbidden:
		httpx.WriteError(w, http.StatusBadGateway, fmt.Sprintf("%s (код %d)", fallback, apiErr.Status))
	case http.StatusNotFound:
		httpx.WriteError(w, http.StatusNotFound, message)
	case http.StatusConflict:
		httpx.WriteError(w, http.StatusConflict, message)
	default:
		httpx.WriteError(w, http.StatusBadGateway, fmt.Sprintf("%s (код %d)", fallback, apiErr.Status))
	}
}
