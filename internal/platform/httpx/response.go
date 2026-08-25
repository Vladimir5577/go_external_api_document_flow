package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"go_external_api_document_flow/internal/client"
)

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("Ошибка сериализации ответа", "error", err)
	}
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]any{"error": message})
}

func asAPIError(err error) *client.APIError {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

// Unavailable formats the legacy Symfony-style "<prefix> (код N)" error.
func Unavailable(err error, prefix string) string {
	apiErr := asAPIError(err)
	if apiErr == nil || apiErr.Status == 0 {
		return prefix + " (нет ответа)"
	}
	return fmt.Sprintf("%s (код %d)", prefix, apiErr.Status)
}

func NotFoundOr(err error, notFound, prefix string) string {
	if apiErr := asAPIError(err); apiErr != nil && apiErr.Status == http.StatusNotFound {
		return notFound
	}
	return Unavailable(err, prefix)
}

func SaveError(err error, notFound, failedPrefix string, withViolations bool) string {
	apiErr := asAPIError(err)
	if apiErr == nil || apiErr.Status == 0 {
		return failedPrefix + " (нет ответа)"
	}

	switch apiErr.Status {
	case http.StatusNotFound:
		return notFound
	case http.StatusUnprocessableEntity:
		message := apiErr.Message()
		if message == "" {
			message = "Ошибка валидации"
		}
		if withViolations {
			if violations := apiErr.Violations(); violations != "" {
				message += ": " + violations
			}
		}
		return message
	default:
		return fmt.Sprintf("%s (код %d)", failedPrefix, apiErr.Status)
	}
}
