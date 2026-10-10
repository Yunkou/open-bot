package db

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Auto-pick mode enum (channel settings + org feature_flags).
const (
	AutoPickOff       = "off"
	AutoPickHeuristic = "heuristic"
	AutoPickDecision  = "decision"
)

// ChannelSettings is stored in channels.settings_json.
// Client advanced settings: show_reply_candidates ("本轮将由谁回答", default off).
type ChannelSettings struct {
	// AutoPick: "" = inherit org (default heuristic); "off" | "heuristic" | "decision".
	// Legacy bool true/omit→heuristic, false→off (see ParseAutoPickMode / parseChannelSettingsJSON).
	AutoPick string `json:"auto_pick,omitempty"`
	// AutoPickK: 0 = default 1; capped at 2–3 by API.
	AutoPickK int `json:"auto_pick_k,omitempty"`
	// ShowReplyCandidates: debug toggle for「本轮将由谁回答」; default false.
	ShowReplyCandidates bool `json:"show_reply_candidates,omitempty"`
}

// ParseAutoPickMode normalizes API / stored values to off|heuristic|decision.
// Legacy: true / "true" / "on" / "1" → heuristic; false / "false" / "0" / "no" → off.
// Empty / omit → "" (inherit). Unknown non-empty → error.
func ParseAutoPickMode(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	switch t := raw.(type) {
	case bool:
		if t {
			return AutoPickHeuristic, nil
		}
		return AutoPickOff, nil
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "":
			return "", nil
		case AutoPickOff, "false", "0", "no":
			return AutoPickOff, nil
		case AutoPickHeuristic, "true", "on", "1", "a", "auto":
			return AutoPickHeuristic, nil
		case AutoPickDecision, "b", "llm", "model":
			return AutoPickDecision, nil
		default:
			return "", fmt.Errorf("auto_pick must be off|heuristic|decision")
		}
	case float64:
		if t == 0 {
			return AutoPickOff, nil
		}
		return AutoPickHeuristic, nil
	case json.Number:
		n, err := t.Float64()
		if err != nil {
			return "", fmt.Errorf("auto_pick must be off|heuristic|decision")
		}
		if n == 0 {
			return AutoPickOff, nil
		}
		return AutoPickHeuristic, nil
	default:
		return "", fmt.Errorf("auto_pick must be off|heuristic|decision")
	}
}

// NormalizeAutoPickMode returns a concrete mode (never empty). Default heuristic.
func NormalizeAutoPickMode(mode string) string {
	s := strings.ToLower(strings.TrimSpace(mode))
	switch s {
	case AutoPickOff:
		return AutoPickOff
	case AutoPickDecision:
		return AutoPickDecision
	case AutoPickHeuristic, "":
		return AutoPickHeuristic
	default:
		// Best-effort legacy strings already normalized by ParseAutoPickMode.
		if m, err := ParseAutoPickMode(s); err == nil && m != "" {
			return m
		}
		return AutoPickHeuristic
	}
}

// EffectiveAutoPickMode resolves channel override vs org mode.
// Channel empty → org; org empty → heuristic. Always returns off|heuristic|decision.
func (s ChannelSettings) EffectiveAutoPickMode(orgMode string) string {
	if strings.TrimSpace(s.AutoPick) != "" {
		return NormalizeAutoPickMode(s.AutoPick)
	}
	return NormalizeAutoPickMode(orgMode)
}

// EffectiveAutoPick reports whether AutoPick should run (any mode other than off).
// Deprecated name kept for call-site clarity during enum migration.
func (s ChannelSettings) EffectiveAutoPick(orgEnabled bool) bool {
	org := AutoPickOff
	if orgEnabled {
		org = AutoPickHeuristic
	}
	return s.EffectiveAutoPickMode(org) != AutoPickOff
}

func parseChannelSettingsJSON(raw string) ChannelSettings {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ChannelSettings{}
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		return ChannelSettings{}
	}
	var s ChannelSettings
	if v, ok := rawMap["auto_pick"]; ok && string(v) != "null" {
		var anyVal any
		if err := json.Unmarshal(v, &anyVal); err == nil {
			if mode, err := ParseAutoPickMode(anyVal); err == nil {
				s.AutoPick = mode
			}
		}
	}
	if v, ok := rawMap["auto_pick_k"]; ok {
		_ = json.Unmarshal(v, &s.AutoPickK)
	}
	if v, ok := rawMap["show_reply_candidates"]; ok {
		_ = json.Unmarshal(v, &s.ShowReplyCandidates)
	}
	return s
}

func (s ChannelSettings) JSON() string {
	b, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	if string(b) == "null" {
		return "{}"
	}
	return string(b)
}

func (d *DB) migrateChannelSettings() error {
	_, err := d.SQL.Exec(`ALTER TABLE channels ADD COLUMN IF NOT EXISTS settings_json TEXT NOT NULL DEFAULT '{}'`)
	return err
}

// UpdateChannelSettings merges patch into channel settings_json (non-nil fields win).
// setAutoPick writes patch.AutoPick (already normalized off|heuristic|decision, or "" to clear inherit).
func (d *DB) UpdateChannelSettings(userID, channelID string, patch ChannelSettings, setAutoPick, setK, setShow bool) (*Channel, error) {
	ch, err := d.GetChannel(userID, channelID)
	if err != nil {
		return nil, err
	}
	cur := ch.Settings
	if setAutoPick {
		cur.AutoPick = patch.AutoPick
	}
	if setK {
		k := patch.AutoPickK
		if k < 0 {
			k = 0
		}
		if k > 3 {
			k = 3
		}
		cur.AutoPickK = k
	}
	if setShow {
		cur.ShowReplyCandidates = patch.ShowReplyCandidates
	}
	_, err = d.SQL.Exec(
		`UPDATE channels SET settings_json = $1 WHERE id = $2 AND user_id = $3`,
		cur.JSON(), channelID, userID,
	)
	if err != nil {
		return nil, err
	}
	return d.GetChannel(userID, channelID)
}
