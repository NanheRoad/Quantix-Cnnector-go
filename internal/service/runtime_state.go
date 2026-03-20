package service

import (
	"time"
)

type RuntimeState struct {
	DeviceID       uint
	DeviceName     string
	DeviceCode     string
	DeviceCategory string
	EventType      string
	Payload        map[string]any
	Status         string
	Weight         *float64
	Unit           string
	LastUpdate     *time.Time
	Error          *string
	StepResults    map[string]any
}

func NewRuntimeState(deviceID uint, name, code, category string) *RuntimeState {
	return &RuntimeState{
		DeviceID:       deviceID,
		DeviceName:     name,
		DeviceCode:     code,
		DeviceCategory: category,
		EventType:      "weight_update",
		Payload:        map[string]any{},
		Status:         "offline",
		Unit:           "kg",
		StepResults:    map[string]any{},
	}
}

func (r *RuntimeState) ToMessage() map[string]any {
	var ts any
	if r.LastUpdate != nil {
		ts = r.LastUpdate.UTC().Format(time.RFC3339Nano)
	}
	var errVal any
	if r.Error != nil {
		errVal = *r.Error
	}
	out := map[string]any{
		"type":           safeString(r.EventType, "weight_update"),
		"device_id":      r.DeviceID,
		"device_name":    r.DeviceName,
		"device_code":    r.DeviceCode,
		"device_category": r.DeviceCategory,
		"weight":         r.Weight,
		"unit":           safeString(r.Unit, "kg"),
		"payload":        cloneMap(r.Payload),
		"timestamp":      ts,
		"status":         safeString(r.Status, "offline"),
		"error":          errVal,
	}
	for k, v := range r.Payload {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func (r *RuntimeState) MarkOnline(weight *float64, unit, eventType string, payload map[string]any) {
	now := time.Now().UTC()
	r.Status = "online"
	r.Weight = weight
	r.Unit = safeString(unit, "kg")
	r.EventType = safeString(eventType, "weight_update")
	r.Payload = cloneMap(payload)
	r.Error = nil
	r.LastUpdate = &now
}

func (r *RuntimeState) MarkOffline(errText string) {
	now := time.Now().UTC()
	r.Status = "offline"
	r.EventType = "weight_update"
	r.Payload = map[string]any{}
	if errText != "" {
		r.Error = &errText
	} else {
		r.Error = nil
	}
	r.LastUpdate = &now
}

func (r *RuntimeState) MarkError(errText string) {
	now := time.Now().UTC()
	r.Status = "error"
	r.EventType = "weight_update"
	r.Payload = map[string]any{}
	r.Error = &errText
	r.LastUpdate = &now
}

func cloneMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func safeString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
