package httpserver

import "strings"

// clientEnvMachineID reads client.machine_id from the send-message client envelope.
func clientEnvMachineID(client map[string]any) string {
	if client == nil {
		return ""
	}
	raw, ok := client["machine_id"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(strings.TrimSpace(stringifyClientScalar(v)))
	}
}

func stringifyClientScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers are rare for ids; ignore.
		return ""
	default:
		return ""
	}
}
