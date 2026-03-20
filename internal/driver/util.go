package driver

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

func asString(m map[string]any, key, fallback string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	return fmt.Sprintf("%v", v)
}

func asInt(m map[string]any, key string, fallback int) int {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n)
		}
		if f, err := t.Float64(); err == nil {
			return int(f)
		}
		return fallback
	case string:
		s := strings.TrimSpace(t)
		n, err := strconv.Atoi(s)
		if err == nil {
			return n
		}
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr == nil {
			return int(f)
		}
		return fallback
	default:
		return fallback
	}
}

func asFloat(m map[string]any, key string, fallback float64) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return fallback
		}
		return n
	default:
		return fallback
	}
}

func asBool(m map[string]any, key string, fallback bool) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return fallback
	}
	switch t := v.(type) {
	case bool:
		return t
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	default:
		return fallback
	}
}

func toBytes(data any, encoding string) ([]byte, error) {
	switch v := data.(type) {
	case []byte:
		return v, nil
	case string:
		if strings.ToLower(strings.TrimSpace(encoding)) == "hex" {
			clean := strings.ReplaceAll(v, " ", "")
			return hex.DecodeString(clean)
		}
		return []byte(decodeEscapes(v)), nil
	default:
		s := fmt.Sprintf("%v", v)
		if strings.ToLower(strings.TrimSpace(encoding)) == "hex" {
			clean := strings.ReplaceAll(s, " ", "")
			return hex.DecodeString(clean)
		}
		return []byte(decodeEscapes(s)), nil
	}
}

func decodeEscapes(text string) string {
	r := strings.NewReplacer(`\r`, "\r", `\n`, "\n", `\t`, "\t")
	return r.Replace(text)
}
