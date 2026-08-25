package main

import (
	"log/slog"
	"os"

	"go_external_api_document_flow/internal/app"
	"go_external_api_document_flow/internal/config"
	"go_external_api_document_flow/internal/logger"
)

func main() {
	cfg := config.Load()

	logger.Setup(cfg.App.Env)

	application, err := app.NewApp(cfg)
	if err != nil {
		slog.Error("Can't initialize application", "error", err)
		os.Exit(1)
	}

	slog.Info("Микросервис внешних API запускается", "port", cfg.App.Port, "upstream", cfg.DonSnab.APIURL)
	if err := application.Run(); err != nil {
		slog.Error("Ошибка старта HTTP-сервера", "error", err)
		os.Exit(1)
	}
}
