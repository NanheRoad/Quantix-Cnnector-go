package api

import (
	"regexp"
	"strings"

	"quantix-connector-go/internal/store"
)

var deviceCodeRegex = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,63}$`)

type ProtocolTemplateCreate struct {
	Name         string         `json:"name" binding:"required"`
	Description  string         `json:"description"`
	ProtocolType string         `json:"protocol_type" binding:"required"`
	Template     map[string]any `json:"template" binding:"required"`
	IsSystem     bool           `json:"is_system"`
}

type ProtocolTemplateUpdate struct {
	Name         *string        `json:"name"`
	Description  *string        `json:"description"`
	ProtocolType *string        `json:"protocol_type"`
	Template     map[string]any `json:"template"`
}

type DeviceCreate struct {
	DeviceCode         string         `json:"device_code" binding:"required"`
	DeviceCategory     string         `json:"device_category"`
	Name               string         `json:"name" binding:"required"`
	ProtocolTemplateID uint           `json:"protocol_template_id" binding:"required"`
	ConnectionParams   map[string]any `json:"connection_params"`
	TemplateVariables  map[string]any `json:"template_variables"`
	PollInterval       float64        `json:"poll_interval"`
	Enabled            *bool          `json:"enabled"`
}

type DeviceUpdate struct {
	DeviceCode         *string        `json:"device_code"`
	DeviceCategory     *string        `json:"device_category"`
	Name               *string        `json:"name"`
	ProtocolTemplateID *uint          `json:"protocol_template_id"`
	ConnectionParams   map[string]any `json:"connection_params"`
	TemplateVariables  map[string]any `json:"template_variables"`
	PollInterval       *float64       `json:"poll_interval"`
	Enabled            *bool          `json:"enabled"`
}

type DeviceConnectionTestRequest struct {
	ProtocolTemplateID uint           `json:"protocol_template_id" binding:"required"`
	ConnectionParams   map[string]any `json:"connection_params" binding:"required"`
	TimeoutMS          int            `json:"timeout_ms"`
}

type ExecuteStepRequest struct {
	StepID string         `json:"step_id" binding:"required"`
	Params map[string]any `json:"params"`
}

type ProtocolTestRequest struct {
	ConnectionParams  map[string]any `json:"connection_params"`
	TemplateVariables map[string]any `json:"template_variables"`
}

type StepTestRequest struct {
	ConnectionParams  map[string]any            `json:"connection_params"`
	TemplateVariables map[string]any            `json:"template_variables"`
	StepID            string                    `json:"step_id" binding:"required"`
	StepContext       string                    `json:"step_context" binding:"required"`
	AllowWrite        bool                      `json:"allow_write"`
	TestPayload       *string                   `json:"test_payload"`
	PreviousSteps     map[string]map[string]any `json:"previous_steps"`
}

type PrintRequest struct {
	StepID *string        `json:"step_id"`
	Params map[string]any `json:"params"`
}

type SerialOpenRequest struct {
	Port      string  `json:"port" binding:"required"`
	Baudrate  int     `json:"baudrate"`
	Bytesize  int     `json:"bytesize"`
	Parity    string  `json:"parity"`
	Stopbits  float64 `json:"stopbits"`
	TimeoutMS int     `json:"timeout_ms"`
}

type SerialSendRequest struct {
	Data       string `json:"data" binding:"required"`
	DataFormat string `json:"data_format"`
	Encoding   string `json:"encoding"`
	LineEnding string `json:"line_ending"`
}

func validateDeviceCode(raw string) error {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !deviceCodeRegex.MatchString(code) {
		return errf("device_code must match ^[A-Z0-9][A-Z0-9_-]{0,63}$")
	}
	return nil
}

func validateDeviceCategory(raw string) error {
	c := strings.ToLower(strings.TrimSpace(raw))
	if c == "" {
		return nil
	}
	if _, ok := store.DeviceCategoryOptions[c]; !ok {
		return errf("device_category must be one of [printer_tsc scanner serial_board weight]")
	}
	return nil
}

type simpleErr struct{ s string }

func (e simpleErr) Error() string { return e.s }
func errf(s string) error         { return simpleErr{s: s} }
