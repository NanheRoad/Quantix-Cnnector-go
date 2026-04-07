package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	PrintAgent            PrintAgentSettings
}

type PrintAgentSettings struct {
	Enabled            bool              `json:"enabled"`
	ServerURL          string            `json:"server_url"`
	AgentAPIKey        string            `json:"agent_api_key"`
	ClientID           string            `json:"client_id"`
	JobType            string            `json:"job_type"`
	DefaultPrinterName string            `json:"default_printer_name"`
	BartenderExecutable string           `json:"bartender_executable"`
	PollIntervalMS     int               `json:"poll_interval_ms"`
	TemplateMappings   map[string]string `json:"template_mappings"`
}

func Load() Settings {
	local := loadLocalConfig()
	defaultAPIKey := "quantix-dev-key"
	if strings.TrimSpace(local.APIKey) != "" {
		defaultAPIKey = strings.TrimSpace(local.APIKey)
	}
	return Settings{
		DBType:                strings.ToLower(getenv("DB_TYPE", "sqlite")),
		DBName:                getenv("DB_NAME", "quantix.db"),
		DBUser:                getenv("DB_USER", ""),
		DBPassword:            getenv("DB_PASSWORD", ""),
		DBHost:                getenv("DB_HOST", "127.0.0.1"),
		DBPort:                atoi(getenv("DB_PORT", "3306"), 3306),
		APIKey:                getenv("API_KEY", defaultAPIKey),
		LogLevel:              getenv("LOG_LEVEL", "INFO"),
		BackendHost:           getenv("BACKEND_HOST", "127.0.0.1"),
		BackendPort:           atoi(getenv("BACKEND_PORT", "8000"), 8000),
		FrontendHost:          getenv("FRONTEND_HOST", "127.0.0.1"),
		FrontendPort:          atoi(getenv("FRONTEND_PORT", "8001"), 8001),
		SimulateOnConnectFail: parseBool(getenv("SIMULATE_ON_CONNECT_FAIL", "false")),
		PrintAgent:            normalizePrintAgentSettings(local.PrintAgent),
	}
}

type localConfig struct {
	APIKey     string             `json:"api_key"`
	PrintAgent PrintAgentSettings `json:"print_agent"`
}

func localConfigPath() string {
	return filepath.Join(".", "quantix.local.json")
}

func loadLocalConfig() localConfig {
	path := localConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return localConfig{}
	}
	var cfg localConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return localConfig{}
	}
	return cfg
}

func SaveAPIKey(apiKey string) error {
	cfg := loadLocalConfig()
	cfg.APIKey = strings.TrimSpace(apiKey)
	return saveLocalConfig(cfg)
}

func SavePrintAgentSettings(settings PrintAgentSettings) error {
	path := localConfigPath()
	cfg := loadLocalConfig()
	cfg.PrintAgent = normalizePrintAgentSettings(settings)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func saveLocalConfig(cfg localConfig) error {
	path := localConfigPath()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizePrintAgentSettings(in PrintAgentSettings) PrintAgentSettings {
	out := in
	out.ServerURL = strings.TrimRight(strings.TrimSpace(out.ServerURL), "/")
	out.AgentAPIKey = strings.TrimSpace(out.AgentAPIKey)
	out.ClientID = strings.TrimSpace(out.ClientID)
	out.JobType = strings.TrimSpace(out.JobType)
	if out.JobType == "" {
		out.JobType = "bartender"
	}
	out.DefaultPrinterName = strings.TrimSpace(out.DefaultPrinterName)
	out.BartenderExecutable = strings.TrimSpace(out.BartenderExecutable)
	if out.PollIntervalMS <= 0 {
		out.PollIntervalMS = 2000
	}
	if out.TemplateMappings == nil {
		out.TemplateMappings = map[string]string{}
	}
	cleaned := make(map[string]string, len(out.TemplateMappings))
	for k, v := range out.TemplateMappings {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		cleaned[key] = val
	}
	out.TemplateMappings = cleaned
	return out
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
