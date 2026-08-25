package vacancyapplications

import (
	"net/http"
	"net/url"
	"strconv"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/platform/httpx"
)

const vacancyApplicationsPath = "/api/vacancy-applications"

type VacancyApplicationHandler struct {
	api       *client.DonSnab
	presenter *dto.Presenter
}

func NewVacancyApplicationHandler(api *client.DonSnab, presenter *dto.Presenter) *VacancyApplicationHandler {
	return &VacancyApplicationHandler{api: api, presenter: presenter}
}

func (h *VacancyApplicationHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := url.Values{}
		query.Set("page", httpx.PageParam(r))
		query.Set("limit", httpx.ListLimit)
		httpx.CopyFilters(query, r, "status", "vacancyId", "search", "dateFrom", "dateTo")
		query.Set("sort", httpx.QueryOr(r, "sort", "createdAt"))
		query.Set("order", httpx.QueryOr(r, "order", "desc"))

		raw, err := h.api.GetJSON(r.Context(), vacancyApplicationsPath, query)
		if err != nil {
			httpx.WriteError(w, http.StatusBadGateway, httpx.Unavailable(err, "Сервис откликов недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, h.presenter.VacancyApplicationList(raw))
	}
}

func (h *VacancyApplicationHandler) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Отклик не найден")
			return
		}

		application, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Отклик не найден", "Сервис откликов недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": application})
	}
}

func (h *VacancyApplicationHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Отклик не найден")
			return
		}

		body, err := httpx.DecodeBody(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		payload := httpx.PickPresent(body, "status", "adminComment")
		if _, err := h.api.SendJSON(r.Context(), http.MethodPatch, vacancyApplicationsPath+"/"+strconv.FormatInt(id, 10), payload, 0); err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.SaveError(err, "Отклик не найден", "Не удалось сохранить изменения", false))
			return
		}

		application, err := h.getOne(r, id)
		if err != nil {
			httpx.WriteError(w, http.StatusUnprocessableEntity, httpx.NotFoundOr(err, "Отклик не найден", "Сервис откликов недоступен"))
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": application})
	}
}

func (h *VacancyApplicationHandler) Resume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.IDParam(r, "id")
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "Резюме не найдено")
			return
		}

		httpx.NoStore(w)
		path := vacancyApplicationsPath + "/" + strconv.FormatInt(id, 10) + "/resume"
		if err := h.api.Stream(r.Context(), w, path, httpx.DownloadParam(r)); err != nil {
			httpx.WriteError(w, http.StatusNotFound, httpx.NotFoundOr(err, "Резюме не найдено", "Ошибка получения резюме"))
			return
		}
	}
}

func (h *VacancyApplicationHandler) getOne(r *http.Request, id int64) (map[string]any, error) {
	raw, err := h.api.GetJSON(r.Context(), vacancyApplicationsPath+"/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	return h.presenter.VacancyApplication(dto.Data(raw), true), nil
}
