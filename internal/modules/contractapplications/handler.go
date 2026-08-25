package contractapplications

import (
	"net/http"
	"net/url"
	"strconv"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/pdf"
	"go_external_api_document_flow/internal/platform/httpx"
)

const contractApplicationsPath = "/api/contract-applications"

type ContractApplicationHandler struct {
	api       *client.DonSnab
	presenter *dto.Presenter
	renderer  *pdf.Renderer
}

func NewContractApplicationHandler(api *client.DonSnab, presenter *dto.Presenter, renderer *pdf.Renderer) *ContractApplicationHandler {
	return &ContractApplicationHandler{api: api, presenter: presenter, renderer: renderer}
}

func (h *ContractApplicationHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := url.Values{}
		query.Set("page", httpx.PageParam(r))
		query.Set("limit", httpx.ListLimit)
		httpx.CopyFilters(query, r, "status", "consumerType", "dateFrom", "dateTo", "search")
		query.Set("sort", httpx.QueryOr(r, "sort", "id"))
		query.Set("order", httpx.QueryOr(r, "order", "desc"))

		raw, err := h.api.GetJSON(r.Context(), contractApplicationsPath, query)
		if err != nil {
			httpx.WriteError(w, http.StatusBadGateway, httpx.Unavailable(err, "Сервис заявок недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.ContractApplicationList(raw))
	}
}

func (h *ContractApplicationHandler) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Заявка не найдена")
			return
		}

		application, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Заявка не найдена", "Сервис заявок недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": application})
	}
}

func (h *ContractApplicationHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Заявка не найдена")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		payload := httpx.PickPresent(body, "status", "adminComment")
		if _, err := h.api.SendJSON(r.Context(), http.MethodPatch, contractApplicationsPath+"/"+strconv.FormatInt(id, 10), payload, 0); err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.SaveError(err, "Заявка не найдена", "Не удалось сохранить изменения", false))
			return
		}

		application, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.NotFoundOr(err, "Заявка не найдена", "Сервис заявок недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": application})
	}
}

func (h *ContractApplicationHandler) File() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Файл не найден")
			return
		}

		httpx.NoStore(w)
		path := contractApplicationsPath + "/files/" + strconv.FormatInt(id, 10)
		if err := h.api.Stream(r.Context(), w, path, httpx.DownloadParam(r)); err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Файл не найден", "Ошибка получения файла"))
			return
		}
	}
}

// PDF — порт ContractApplicationController::pdf() из Twig-слоя монолита.
func (h *ContractApplicationHandler) PDF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Заявка не найдена")
			return
		}

		raw, err := h.api.GetJSON(r.Context(), contractApplicationsPath+"/"+strconv.FormatInt(id, 10), nil)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Заявка не найдена", "Сервис заявок недоступен"))
			return
		}

		application := dto.Data(raw)
		files := httpx.FetchAttachments(r.Context(), h.api, application["files"], func(fileID int64) string {
			return contractApplicationsPath + "/files/" + strconv.FormatInt(fileID, 10)
		})

		content, err := h.renderer.ContractApplication(r.Context(), application, files)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Не удалось сформировать PDF")
			return
		}

		httpx.WritePDF(w, "application-"+httpx.StrOf(application["publicId"])+".pdf", content)
	}
}

func (h *ContractApplicationHandler) getOne(r *http.Request, id int64) (map[string]any, error) {
	raw, err := h.api.GetJSON(r.Context(), contractApplicationsPath+"/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	return h.presenter.ContractApplication(dto.Data(raw), true), nil
}
