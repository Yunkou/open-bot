package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleGetUserSettings(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	settings, err := s.db.GetUserSettings(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutUserSettings(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body db.UserSettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.Timezone == nil && body.AutoReviewEnabled == nil && body.AutoReviewRules == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}
	settings, err := s.db.UpsertUserSettings(uid, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) userSettingsOrDefault(uid string) db.UserSettings {
	settings, err := s.db.GetUserSettings(uid)
	if err != nil {
		return db.DefaultUserSettings(uid)
	}
	return settings
}

func effectiveTimezone(settings db.UserSettings) string {
	return strings.TrimSpace(settings.Timezone)
}

// attachUserTimezone merges the user's preferred timezone into the runtime client envelope
// so the agent environment block can show the correct local time zone.
func attachUserTimezone(payload map[string]any, settings db.UserSettings) {
	tz := effectiveTimezone(settings)
	if tz == "" || payload == nil {
		return
	}
	client, _ := payload["client"].(map[string]any)
	if client == nil {
		client = map[string]any{}
	}
	if existing, _ := client["timezone"].(string); strings.TrimSpace(existing) != "" {
		return
	}
	client["timezone"] = tz
	payload["client"] = client
}
