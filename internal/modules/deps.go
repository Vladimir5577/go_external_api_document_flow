package modules

import (
	"time"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/pdf"
)

// Deps contains shared application services available to modules.
// Module-specific clients/configs should stay inside the module package.
type Deps struct {
	DonSnab   *client.DonSnab
	Presenter *dto.Presenter
	Renderer  *pdf.Renderer
	Location  *time.Location
}
