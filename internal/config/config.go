package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App     AppConfig
	Auth    AuthConfig
	DonSnab DonSnabConfig
	PDF     PDFConfig
}

type AppConfig struct {
	Env      string
	Port     string
	Location *time.Location
}

type AuthConfig struct {
	JWTPublicKeyPath string
}

// DonSnabConfig configures the shared don_snab upstream client used by current modules.
type DonSnabConfig struct {
	APIURL  string
	APIKey  string
	Timeout time.Duration
}

// PDFConfig configures the shared renderer dependency.
type PDFConfig struct {
	FontRegularPath string
	FontBoldPath    string
}

func Load() *Config {
	// Пытаемся загрузить локальный .env файл, если он есть
	if err := godotenv.Load(); err != nil {
		slog.Warn("Предупреждение: .env файл не найден, используются системные переменные окружения")
	}

	tz := GetEnv("APP_TIMEZONE", "Europe/Moscow")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		slog.Warn("Неизвестная таймзона, используется UTC", "timezone", tz, "error", err)
		loc = time.UTC
	}

	c := &Config{
		App: AppConfig{
			Env:      GetEnv("ENV", "local"),
			Port:     GetEnv("SERVER_PORT", "8087"),
			Location: loc,
		},
		Auth: AuthConfig{
			JWTPublicKeyPath: GetEnv("JWT_PUBLIC_KEY_PATH", "config/jwt/public.pem"),
		},
		DonSnab: DonSnabConfig{
			APIURL:  GetEnv("DON_SNAB_API_URL", ""),
			APIKey:  GetEnv("DON_SNAB_API_KEY", ""),
			Timeout: time.Duration(GetEnvAsInt("UPSTREAM_TIMEOUT_SECONDS", 30)) * time.Second,
		},
		PDF: PDFConfig{
			FontRegularPath: GetEnv("PDF_FONT_REGULAR", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"),
			FontBoldPath:    GetEnv("PDF_FONT_BOLD", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"),
		},
	}

	if c.DonSnab.APIURL == "" {
		slog.Warn("DON_SNAB_API_URL не задан — все запросы к внешнему сервису будут падать")
	}

	return c
}

func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func GetEnvAsInt(key string, fallback int) int {
	valueStr := GetEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}
