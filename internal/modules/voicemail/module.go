// Package voicemail проксирует запросы в микросервис голосовой почты.
//
// Раньше модуль сам ходил на АТС и переименовывал поля ответа. Теперь на АТС
// ходит go_voicemail_service_document_flow: он хранит обращения у себя, поэтому
// у них появились статус и комментарий администратора, которых у Asterisk нет.
//
// Шлюз тело ответа не разбирает: новые поля и фильтры микросервиса доезжают до
// фронта, не требуя правок здесь.
package voicemail

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"go_external_api_document_flow/internal/config"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/platform/httpx"

	"github.com/go-chi/chi/v5"
)

type Module struct {
	proxy http.Handler
}

func New() *Module {
	target := config.GetEnv("VOICEMAIL_SERVICE_URL", "")
	if target == "" {
		slog.Warn("VOICEMAIL_SERVICE_URL не задан — модуль голосовой почты будет недоступен")
		return &Module{proxy: unavailable()}
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		slog.Error("VOICEMAIL_SERVICE_URL не разобран", "url", target, "error", err)
		return &Module{proxy: unavailable()}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Путь не режем: микросервис слушает ровно на этом префиксе.
			pr.SetURL(parsed)
			pr.Out.Host = parsed.Host
			pr.SetXForwarded()

			// Микросервис своей авторизации не имеет и заголовкам доверяет,
			// потому что живёт в закрытой сети. Значит подделать их не должно
			// быть возможности: сначала выкидываем то, что прислал клиент, и
			// только потом ставим разобранное из подписанного JWT.
			pr.Out.Header.Del("X-User-Id")
			pr.Out.Header.Del("X-User-Name")

			if user, ok := middleware.GetUser(pr.In.Context()); ok {
				pr.Out.Header.Set("X-User-Id", strconv.FormatInt(user.ID, 10))
				pr.Out.Header.Set("X-User-Name", user.Username)
			}
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("Микросервис голосовой почты недоступен", "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "Сервис голосовой почты недоступен")
		},
	}

	return &Module{proxy: proxy}
}

func (m *Module) Name() string {
	return "voicemail"
}

func (m *Module) Mount(r chi.Router) {
	r.Route(dto.APIPrefix+"/voicemail", func(r chi.Router) {
		r.Use(middleware.RequireRole("ROLE_CITIZEN_APPEAL"))

		// Один маршрут вместо пяти: что именно умеет микросервис, шлюз не знает.
		r.Handle("/*", m.proxy)
	})
}

func unavailable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusBadGateway, "Сервис голосовой почты не настроен")
	})
}
