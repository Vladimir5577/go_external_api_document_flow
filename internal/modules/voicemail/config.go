package voicemail

import (
	"log/slog"
	"time"

	"go_external_api_document_flow/internal/config"
)

type Config struct {
	URL     string
	Token   string
	Timeout time.Duration
}

func LoadConfig() Config {
	cfg := Config{
		URL:     config.GetEnv("VMAPI_URL", ""),
		Token:   config.GetEnv("VMAPI_TOKEN", ""),
		Timeout: time.Duration(config.GetEnvAsInt("VMAPI_TIMEOUT_SECONDS", 60)) * time.Second,
	}

	if cfg.URL == "" {
		slog.Warn("VMAPI_URL не задан — модуль голосовой почты будет недоступен")
	}
	if cfg.Token == "" {
		slog.Warn("VMAPI_TOKEN не задан — запросы к голосовой почте будут отклонены")
	}

	return cfg
}
