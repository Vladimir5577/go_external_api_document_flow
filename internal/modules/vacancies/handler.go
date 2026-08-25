package vacancies

import (
	"net/http"
	"net/url"
	"strconv"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/platform/httpx"
)

const vacanciesPath = "/api/vacancies"

type VacancyHandler struct {
	api       *client.DonSnab
	presenter *dto.Presenter
}

func NewVacancyHandler(api *client.DonSnab, presenter *dto.Presenter) *VacancyHandler {
	return &VacancyHandler{api: api, presenter: presenter}
}

func (h *VacancyHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := url.Values{}
		query.Set("page", httpx.PageParam(r))
		query.Set("limit", httpx.ListLimit)
		httpx.CopyFilters(query, r, "isPublished", "city", "employmentType", "schedule", "experience", "search")
		query.Set("sort", httpx.QueryOr(r, "sort", "sortOrder"))
		query.Set("order", httpx.QueryOr(r, "order", "asc"))

		raw, err := h.api.GetJSON(r.Context(), vacanciesPath, query)
		if err != nil {
			httpx.WriteError(w, http.StatusBadGateway, httpx.Unavailable(err, "Сервис вакансий недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.VacancyList(raw))
	}
}

func (h *VacancyHandler) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Вакансия не найдена")
			return
		}

		raw, err := h.api.GetJSON(r.Context(), vacanciesPath+"/"+strconv.FormatInt(id, 10), nil)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Вакансия не найдена", "Сервис вакансий недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.data(raw))
	}
}

func (h *VacancyHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Тело отдаём upstream как есть — валидация вакансии живёт там.
		raw, err := h.api.SendJSON(r.Context(), http.MethodPost, vacanciesPath, body, http.StatusCreated)
		if err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.SaveError(err, "Вакансия не найдена", "Не удалось создать вакансию", true))
			return
		}

		httpx.WriteJSON(w, http.StatusCreated, h.data(raw))
	}
}

func (h *VacancyHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Вакансия не найдена")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		h.patch(w, r, id, body)
	}
}

func (h *VacancyHandler) Publish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Вакансия не найдена")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		published, ok := body["isPublished"]
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "isPublished is required")
			return
		}

		h.patch(w, r, id, map[string]any{"isPublished": httpx.Truthy(published)})
	}
}

func (h *VacancyHandler) patch(w http.ResponseWriter, r *http.Request, id int64, payload map[string]any) {
	raw, err := h.api.SendJSON(r.Context(), http.MethodPatch, vacanciesPath+"/"+strconv.FormatInt(id, 10), payload, 0)
	if err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.SaveError(err, "Вакансия не найдена", "Не удалось сохранить изменения", true))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, h.data(raw))
}

func (h *VacancyHandler) data(raw map[string]any) map[string]any {
	return map[string]any{"data": h.presenter.Vacancy(dto.Data(raw), true)}
}
