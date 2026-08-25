package vacancyapplications

import (
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/modules"

	"github.com/go-chi/chi/v5"
)

type Module struct {
	h *VacancyApplicationHandler
}

func New(deps modules.Deps) *Module {
	return &Module{
		h: NewVacancyApplicationHandler(deps.DonSnab, deps.Presenter),
	}
}

func (m *Module) Name() string {
	return "vacancy-applications"
}

func (m *Module) Mount(r chi.Router) {
	r.Route(dto.APIPrefix+"/vacancy-applications", func(r chi.Router) {
		r.Use(middleware.RequireRole("ROLE_HR"))

		r.Get("/", m.h.List())
		r.Get("/{id}", m.h.Show())
		r.Patch("/{id}", m.h.Update())
		r.Get("/{id}/resume", m.h.Resume())
	})
}
