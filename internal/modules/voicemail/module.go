package voicemail

import (
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/modules"

	"github.com/go-chi/chi/v5"
)

type Module struct {
	h *Handler
}

func New(cfg Config, deps modules.Deps) *Module {
	return &Module{
		h: NewHandler(NewClient(cfg), NewPresenter(deps.Location)),
	}
}

func (m *Module) Name() string {
	return "voicemail"
}

func (m *Module) Mount(r chi.Router) {
	r.Route(dto.APIPrefix+"/voicemail", func(r chi.Router) {
		r.Use(middleware.RequireRole("ROLE_CITIZEN_APPEAL"))

		r.Get("/health", m.h.Health())
		r.Get("/mailboxes", m.h.Mailboxes())
		r.Get("/mailboxes/{mailbox}/messages", m.h.Messages())
		r.Get("/mailboxes/{mailbox}/messages/{id}/audio", m.h.Audio())
		r.Post("/mailboxes/{mailbox}/ack", m.h.Ack())
	})
}
