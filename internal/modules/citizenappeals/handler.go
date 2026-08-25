package citizenappeals

import (
	"net/http"
	"net/url"
	"strconv"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/pdf"
	"go_external_api_document_flow/internal/platform/httpx"
)

const citizenAppealsPath = "/api/citizen-appeals"

type CitizenAppealHandler struct {
	api       *client.DonSnab
	presenter *dto.Presenter
	renderer  *pdf.Renderer
}

func NewCitizenAppealHandler(api *client.DonSnab, presenter *dto.Presenter, renderer *pdf.Renderer) *CitizenAppealHandler {
	return &CitizenAppealHandler{api: api, presenter: presenter, renderer: renderer}
}

func (h *CitizenAppealHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := url.Values{}
		query.Set("page", httpx.PageParam(r))
		query.Set("limit", httpx.ListLimit)
		httpx.CopyFilters(query, r, "status", "city", "appealType", "dateFrom", "dateTo", "search")
		query.Set("sort", httpx.QueryOr(r, "sort", "id"))
		query.Set("order", httpx.QueryOr(r, "order", "desc"))

		raw, err := h.api.GetJSON(r.Context(), citizenAppealsPath, query)
		if err != nil {
			httpx.WriteError(w, http.StatusBadGateway, httpx.Unavailable(err, "Сервис обращений недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.CitizenAppealList(raw))
	}
}

func (h *CitizenAppealHandler) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Обращение не найдено")
			return
		}

		appeal, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Обращение не найдено", "Сервис обращений недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": appeal})
	}
}

func (h *CitizenAppealHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Обращение не найдено")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		payload := httpx.PickPresent(body, "status", "adminComment")
		if _, err := h.api.SendJSON(r.Context(), http.MethodPatch, citizenAppealsPath+"/"+strconv.FormatInt(id, 10), payload, 0); err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.SaveError(err, "Обращение не найдено", "Не удалось сохранить изменения", false))
			return
		}

		// Перечитываем обращение целиком: upstream на PATCH отдаёт не всё.
		appeal, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.NotFoundOr(err, "Обращение не найдено", "Сервис обращений недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": appeal})
	}
}

func (h *CitizenAppealHandler) File() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Файл не найден")
			return
		}

		httpx.NoStore(w)
		path := citizenAppealsPath + "/files/" + strconv.FormatInt(id, 10)
		if err := h.api.Stream(r.Context(), w, path, httpx.DownloadParam(r)); err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Файл не найден", "Ошибка получения файла"))
			return
		}
	}
}

// PDF — порт CitizenAppealController::pdf() из Twig-слоя монолита.
func (h *CitizenAppealHandler) PDF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Обращение не найдено")
			return
		}

		raw, err := h.api.GetJSON(r.Context(), citizenAppealsPath+"/"+strconv.FormatInt(id, 10), nil)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Обращение не найдено", "Сервис обращений недоступен"))
			return
		}

		appeal := dto.Data(raw)
		files := httpx.FetchAttachments(r.Context(), h.api, appeal["files"], func(fileID int64) string {
			return citizenAppealsPath + "/files/" + strconv.FormatInt(fileID, 10)
		})

		content, err := h.renderer.CitizenAppeal(r.Context(), appeal, files)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Не удалось сформировать PDF")
			return
		}

		httpx.WritePDF(w, "appeal-"+httpx.StrOf(appeal["publicId"])+".pdf", content)
	}
}

func (h *CitizenAppealHandler) getOne(r *http.Request, id int64) (map[string]any, error) {
	raw, err := h.api.GetJSON(r.Context(), citizenAppealsPath+"/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	return h.presenter.CitizenAppeal(dto.Data(raw), true), nil
}
