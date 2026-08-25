package app

import (
	"net/http"
	"time"

	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/modules"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func setupRouter(apiModules []modules.Module, authMw *middleware.AuthMiddleware) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestLogger())
	r.Use(chiMiddleware.Recoverer)
	// 60 секунд, а не 15 как в канбане: сюда попадает стриминг вложений и резюме.
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	// public API group
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok", "service": "external-api"}`))
	})

	// protected API group
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)

		for _, m := range apiModules {
			m.Mount(r)
		}
	})

	return r
}
