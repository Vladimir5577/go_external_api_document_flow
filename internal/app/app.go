package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/config"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/middleware"
	"go_external_api_document_flow/internal/modules"
	"go_external_api_document_flow/internal/modules/citizenappeals"
	"go_external_api_document_flow/internal/modules/contractapplications"
	"go_external_api_document_flow/internal/modules/vacancies"
	"go_external_api_document_flow/internal/modules/vacancyapplications"
	"go_external_api_document_flow/internal/modules/voicemail"
	"go_external_api_document_flow/internal/pdf"

	"github.com/go-chi/chi/v5"
)

type App struct {
	router *chi.Mux
	cfg    *config.Config
}

func NewApp(cfg *config.Config) (*App, error) {
	authMw, err := middleware.NewAuthMiddleware(cfg.Auth.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init auth middleware: %w", err)
	}

	api := client.NewDonSnab(cfg.DonSnab)
	presenter := dto.New(cfg.App.Location)

	renderer, err := pdf.NewRenderer(cfg.PDF.FontRegularPath, cfg.PDF.FontBoldPath, presenter)
	if err != nil {
		return nil, fmt.Errorf("failed to init pdf renderer: %w", err)
	}

	deps := modules.Deps{
		DonSnab:   api,
		Presenter: presenter,
		Renderer:  renderer,
		Location:  cfg.App.Location,
	}

	return &App{
		router: setupRouter(buildModules(deps), authMw),
		cfg:    cfg,
	}, nil
}

func buildModules(deps modules.Deps) []modules.Module {
	return []modules.Module{
		citizenappeals.New(deps),
		contractapplications.New(deps),
		vacancies.New(deps),
		vacancyapplications.New(deps),
		voicemail.New(),
	}
}

func (a *App) Run() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", a.cfg.App.Port),
		Handler:      a.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Запускаем сервер в горутине
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Ошибка старта HTTP-сервера", "error", err)
			os.Exit(1)
		}
	}()

	// Ожидаем сигналы ОС для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Получен сигнал завершения, начинаем graceful shutdown...")

	// Даем 5 секунд на завершение текущих запросов
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при остановке сервера: %w", err)
	}

	slog.Info("HTTP-сервер успешно остановлен")
	return nil
}
