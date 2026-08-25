package modules

import "github.com/go-chi/chi/v5"

// Module is a self-contained API area that owns its routes and access rules.
type Module interface {
	Name() string
	Mount(r chi.Router)
}
