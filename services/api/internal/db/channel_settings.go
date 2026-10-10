package db

import (
	"encoding/json"
	"strings"
)

// ChannelSettings is stored in channels.settings_json.
// Client advanced settings: show_reply_candidates ("本轮将由谁回答", default off).
type ChannelSettings struct {
	// AutoPick: nil = inherit org flag (default ON for A-tier); false disables.
	AutoPick *bool `json:"auto_pick,omitempty"`
	// AutoPickK: 0 = default 1; capped at 2–3 by API.
	AutoPickK int `json:"auto_pick_k,omitempty"`
	// ShowReplyCandidates: debug toggle for「本轮将由谁回答」; default false.
	ShowReplyCandidates bool `json:"show_reply_candidates,omitempty"`
}

func (s ChannelSettings) EffectiveAutoPick(orgEnabled bool) bool {
	if s.AutoPick != nil {
		return *s.AutoPick
	}
	return orgEnabled
}

func parseChannelSettingsJSON(raw string) ChannelSettings {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ChannelSettings{}
	}
	var s ChannelSettings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return ChannelSettings{}
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

