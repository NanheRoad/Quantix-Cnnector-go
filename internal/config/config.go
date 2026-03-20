package config

import (
	"os"
	"strconv"
	"strings"
)

type Settings struct {
	DBType                string
	DBName                string
	DBUser                string
	DBPassword            string
	DBHost                string
	DBPort                int
	APIKey                string
	LogLevel              string
	BackendHost           string
	BackendPort           int
	FrontendHost          string
	FrontendPort          int
	SimulateOnConnectFail bool
}

func Load() Settings {
	return Settings{
		DBType:                strings.ToLower(getenv("DB_TYPE", "sqlite")),
		DBName:                getenv("DB_NAME", "quantix.db"),
		DBUser:                getenv("DB_USER", ""),
		DBPassword:            getenv("DB_PASSWORD", ""),
		DBHost:                getenv("DB_HOST", "127.0.0.1"),
		DBPort:                atoi(getenv("DB_PORT", "3306"), 3306),
		APIKey:                getenv("API_KEY", "quantix-dev-key"),
		LogLevel:              getenv("LOG_LEVEL", "INFO"),
		BackendHost:           getenv("BACKEND_HOST", "127.0.0.1"),
		BackendPort:           atoi(getenv("BACKEND_PORT", "8000"), 8000),
		FrontendHost:          getenv("FRONTEND_HOST", "127.0.0.1"),
		FrontendPort:          atoi(getenv("FRONTEND_PORT", "8001"), 8001),
		SimulateOnConnectFail: parseBool(getenv("SIMULATE_ON_CONNECT_FAIL", "false")),
	}
}

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func atoi(v string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

func parseBool(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}
