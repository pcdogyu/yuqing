package apiutil

import (
	"encoding/json"
	"net/http"
	"strings"
)

type HealthPayload struct {
	Service string `json:"service,omitempty"`
	Status  string `json:"status"`
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

func NormalizeHealthStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "ok", "healthy", "normal", "ready":
		return "ok"
	case "working", "busy", "running", "starting", "warming":
		return "working"
	default:
		return "failed"
	}
}

func IsHealthyStatus(value string) bool {
	switch NormalizeHealthStatus(value) {
	case "ok", "working":
		return true
	default:
		return false
	}
}

func NewHealthPayload(service string, status string, message string) HealthPayload {
	normalized := NormalizeHealthStatus(status)
	message = strings.TrimSpace(message)
	if message == "" {
		switch normalized {
		case "working":
			message = "working"
		case "ok":
			message = "ok"
		default:
			message = "failed"
		}
	}
	return HealthPayload{
		Service: strings.TrimSpace(service),
		Status:  normalized,
		Healthy: IsHealthyStatus(normalized),
		Message: message,
	}
}

func WriteHealth(w http.ResponseWriter, service string, status string, message string) {
	WriteJSON(w, http.StatusOK, "ok", NewHealthPayload(service, status, message))
}

func ParseHealthPayload(body []byte) (HealthPayload, bool) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return HealthPayload{}, false
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return HealthPayload{}, false
	}
	data, _ := object["data"].(map[string]any)
	service := firstHealthString(data, object, "service")
	status := firstHealthString(data, object, "status")
	message := firstHealthString(data, object, "message")
	healthy, hasHealthy := firstHealthBool(data, object, "healthy")
	if service == "" && status == "" && message == "" && !hasHealthy {
		return HealthPayload{}, false
	}
	payload := NewHealthPayload(service, status, message)
	if hasHealthy {
		payload.Healthy = healthy
	}
	return payload, true
}

func CoerceHealthPayload(success bool, service string, body []byte, fallbackMessage string) HealthPayload {
	payload, ok := ParseHealthPayload(body)
	if !ok {
		if success {
			return NewHealthPayload(service, "ok", "ok")
		}
		return NewHealthPayload(service, "failed", fallbackMessage)
	}
	if strings.TrimSpace(payload.Service) == "" {
		payload.Service = strings.TrimSpace(service)
	}
	if !success {
		payload.Status = "failed"
		payload.Healthy = false
		if strings.TrimSpace(payload.Message) == "" || strings.EqualFold(strings.TrimSpace(payload.Message), "ok") {
			payload.Message = strings.TrimSpace(fallbackMessage)
		}
		return NewHealthPayload(payload.Service, payload.Status, payload.Message)
	}
	return NewHealthPayload(payload.Service, payload.Status, payload.Message)
}

func firstHealthString(objects ...any) string {
	if len(objects) < 2 {
		return ""
	}
	key, _ := objects[len(objects)-1].(string)
	for _, object := range objects[:len(objects)-1] {
		typed, ok := object.(map[string]any)
		if !ok {
			continue
		}
		value, ok := typed[key]
		if !ok {
			continue
		}
		text := strings.TrimSpace(stringifyHealthValue(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func firstHealthBool(objects ...any) (bool, bool) {
	if len(objects) < 2 {
		return false, false
	}
	key, _ := objects[len(objects)-1].(string)
	for _, object := range objects[:len(objects)-1] {
		typed, ok := object.(map[string]any)
		if !ok {
			continue
		}
		value, ok := typed[key]
		if !ok {
			continue
		}
		switch typedValue := value.(type) {
		case bool:
			return typedValue, true
		case string:
			switch strings.ToLower(strings.TrimSpace(typedValue)) {
			case "true", "1", "yes":
				return true, true
			case "false", "0", "no":
				return false, true
			}
		}
	}
	return false, false
}

func stringifyHealthValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
