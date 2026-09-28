package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

type machineRegisterBody struct {
	MachineKey string `json:"machine_key"`
	Label      string `json:"label"`
	Platform   string `json:"platform"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	App        string `json:"app"`
	AppVersion string `json:"app_version"`
}

type internalListMachinesBody struct {
	UserID string `json:"user_id"`
}

func (s *Server) handleListMachines(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	list, err := s.db.ListMachines(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []db.Machine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": list})
}

func (s *Server) handleRegisterMachine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body machineRegisterBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.MachineKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "machine_key required"})
		return
	}
	m, err := s.db.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: body.MachineKey,
		Label:      body.Label,
		Platform:   body.Platform,
		OS:         body.OS,
		Arch:       body.Arch,
		App:        body.App,
		AppVersion: body.AppVersion,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"machine": m})
}

func (s *Server) handleHeartbeatMachine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	m, err := s.db.HeartbeatMachine(uid, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "machine not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"machine": m})
}

func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.db.DeleteMachine(uid, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "machine not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInternalListMachines(w http.ResponseWriter, r *http.Request) {
	var body internalListMachinesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id required"})
		return
	}
	list, err := s.db.ListMachines(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []db.Machine{}
	}
	// Slim payload for the model / tool.
	slim := make([]map[string]any, 0, len(list))
	for _, m := range list {
		slim = append(slim, map[string]any{
			"id":        m.ID,
			"label":     m.Label,
			"platform":  m.Platform,
			"os":        m.OS,
			"arch":      m.Arch,
			"app":       m.App,
			"status":    m.Status,
			"last_seen": db.FormatTime(m.LastSeen),
			"online":    m.Status == "online",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": slim, "count": len(slim)})
}
