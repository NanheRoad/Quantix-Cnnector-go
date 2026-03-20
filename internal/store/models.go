package store

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/datatypes"
)

var deviceCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,63}$`)

var DeviceCategoryOptions = map[string]struct{}{
	"weight":       {},
	"printer_tsc":  {},
	"scanner":      {},
	"serial_board": {},
}

type ProtocolTemplate struct {
	ID          uint              `gorm:"primaryKey"`
	Name        string            `gorm:"size:100;uniqueIndex;not null"`
	Description string            `gorm:"type:text"`
	ProtocolType string           `gorm:"size:50;not null"`
	Template    datatypes.JSONMap `gorm:"type:json"`
	IsSystem    bool              `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Device struct {
	ID               uint              `gorm:"primaryKey"`
	DeviceCode       string            `gorm:"size:64;uniqueIndex;not null"`
	DeviceCategory   string            `gorm:"size:32;not null;default:weight"`
	Name             string            `gorm:"size:100;uniqueIndex;not null"`
	ProtocolTemplateID uint            `gorm:"not null;index"`
	ProtocolTemplate ProtocolTemplate  `gorm:"constraint:OnDelete:CASCADE"`
	ConnectionParams datatypes.JSONMap `gorm:"type:json"`
	TemplateVariables datatypes.JSONMap `gorm:"type:json"`
	PollInterval     float64           `gorm:"not null;default:1.0"`
	Enabled          bool              `gorm:"not null;default:true"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NormalizeDeviceCode(value string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(value))
	if code == "" {
		return "", fmt.Errorf("device_code is required")
	}
	if !deviceCodePattern.MatchString(code) {
		return "", fmt.Errorf("device_code must match ^[A-Z0-9][A-Z0-9_-]{0,63}$")
	}
	return code, nil
}

func NormalizeDeviceCategory(value string) (string, error) {
	category := strings.ToLower(strings.TrimSpace(value))
	if category == "" {
		return "weight", nil
	}
	if _, ok := DeviceCategoryOptions[category]; !ok {
		return "", fmt.Errorf("device_category must be one of [printer_tsc scanner serial_board weight]")
	}
	return category, nil
}

func BuildDefaultDeviceCode(id uint) string {
	return fmt.Sprintf("DEV-%06d", id)
}

func ToJSONMap(input map[string]any) datatypes.JSONMap {
	if input == nil {
		return datatypes.JSONMap{}
	}
	m := datatypes.JSONMap{}
	for k, v := range input {
		m[k] = v
	}
	return m
}

func JSONMapToMap(in datatypes.JSONMap) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
