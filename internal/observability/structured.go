package observability

import (
	"encoding/json"
	"log"
	"time"
)

func Info(event string, fields map[string]interface{}) {
	logEvent("info", event, fields)
}

func Error(event string, fields map[string]interface{}, err error) {
	payload := cloneFields(fields)
	if err != nil {
		payload["error"] = err.Error()
	}
	logEvent("error", event, payload)
}

func logEvent(level, event string, fields map[string]interface{}) {
	payload := cloneFields(fields)
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	payload["level"] = level
	payload["event"] = event
	raw, err := json.Marshal(payload)
	if err != nil {
		fallback := map[string]interface{}{
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"level": "error",
			"event": "log.marshal_failed",
			"error": err.Error(),
		}
		if fields != nil {
			fallback["event_payload"] = fields
		}
		fallbackRaw, _ := json.Marshal(fallback)
		log.Print(string(fallbackRaw))
		return
	}
	log.Print(string(raw))
}

func cloneFields(fields map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})
	for k, v := range fields {
		payload[k] = v
	}
	return payload
}
