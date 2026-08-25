package citizenappeals

import (
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/modules"

	"github.com/go-chi/chi/v5"
)

type Module struct {
	h *CitizenAppealHandler
}

func New(deps modules.Deps) *Module {
	return &Module{
		h: NewCitizenAppealHandler(deps.DonSnab, deps.Presenter, deps.Renderer),
	}
}

func (m *Module) Name() string {
	return "citizen-appeals"
}

func (m *Module) Mount(r chi.Router) {
	r.Route(dto.APIPrefix+"/citizen-appeals", func(r chi.Router) {
		r.Use(middleware.RequireRole("ROLE_CITIZEN_APPEAL"))

		r.Get("/", m.h.List())
		r.Get("/files/{id}", m.h.File())
		r.Get("/{id}", m.h.Show())
		r.Patch("/{id}", m.h.Update())
		r.Get("/{id}/pdf", m.h.PDF())
	})
}
