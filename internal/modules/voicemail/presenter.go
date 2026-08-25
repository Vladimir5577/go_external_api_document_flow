package voicemail

import (
	"net/url"
	"strconv"
	"time"

	"go_external_api_document_flow/internal/dto"
)

type Presenter struct {
	loc *time.Location
}

func NewPresenter(loc *time.Location) *Presenter {
	return &Presenter{loc: loc}
}

func (p *Presenter) Health(raw map[string]any) map[string]any {
	return map[string]any{
		"status":    raw["status"],
		"mailboxes": intOf(raw["mailboxes"]),
	}
}

func (p *Presenter) Mailboxes(raw map[string]any) map[string]any {
	list, _ := raw["mailboxes"].([]any)
	items := make([]map[string]any, 0, len(list))

	for _, item := range list {
		box, ok := item.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"mailbox":  str(box["mailbox"]),
			"name":     str(box["name"]),
			"newCount": intOf(box["new"]),
			"oldCount": intOf(box["old"]),
		})
	}

	return map[string]any{
		"totalNew": intOf(raw["total_new"]),
		"items":    items,
	}
}

func (p *Presenter) Messages(raw map[string]any) map[string]any {
	list, _ := raw["messages"].([]any)
	items := make([]map[string]any, 0, len(list))

	for _, item := range list {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, p.Message(message))
	}

	return map[string]any{
		"mailbox":   str(raw["mailbox"]),
		"count":     intOf(raw["count"]),
		"truncated": boolOf(raw["truncated"]),
		"items":     items,
	}
}

func (p *Presenter) Message(message map[string]any) map[string]any {
	mailbox := str(message["mailbox"])
	id := str(message["id"])

	out := map[string]any{
		"id":            id,
		"mailbox":       mailbox,
		"folder":        str(message["folder"]),
		"callerNumber":  str(message["caller_number"]),
		"callerName":    str(message["caller_name"]),
		"receivedEpoch": intOf(message["received_epoch"]),
		"receivedAt":    p.atom(message["received_at"]),
		"durationSec":   intOf(message["duration_sec"]),
		"audio":         audio(message["audio"]),
		"audioUrl":      dto.APIPrefix + "/voicemail/mailboxes/" + url.PathEscape(mailbox) + "/messages/" + url.PathEscape(id) + "/audio",
	}

	if err := str(message["audio_error"]); err != "" {
		out["audioError"] = err
	}

	return out
}

func (p *Presenter) Ack(raw map[string]any) map[string]any {
	return map[string]any{
		"mailbox":  str(raw["mailbox"]),
		"moved":    stringList(raw["moved"]),
		"notFound": stringList(raw["not_found"]),
	}
}

func audio(raw any) any {
	a, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	return map[string]any{
		"format":    str(a["format"]),
		"bitrate":   str(a["bitrate"]),
		"sizeBytes": intOf(a["size_bytes"]),
		"base64":    str(a["base64"]),
	}
}

func stringList(raw any) []string {
	list, _ := raw.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s := str(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (p *Presenter) atom(v any) any {
	raw := str(v)
	if raw == "" {
		return v
	}

	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format(time.RFC3339)
		}
	}

	return raw
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
	case int:
		return int64(n)
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
