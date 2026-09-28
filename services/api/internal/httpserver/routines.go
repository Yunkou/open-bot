package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func validateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return errors.New("schedule_cron required")
	}
	_, err := cronParser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron (need 5 fields min hour dom month dow): %w", err)
	}
	return nil
}

func (s *Server) handleListRoutines(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	list, err := s.db.ListRoutines(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.Routine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"routines": list})
}

func (s *Server) handleCreateRoutine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body struct {
		Name         string `json:"name"`
		Prompt       string `json:"prompt"`
		ScheduleCron string `json:"schedule_cron"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := validateCron(body.ScheduleCron); err != nil && strings.TrimSpace(body.ScheduleCron) != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.ScheduleCron) == "" {
		body.ScheduleCron = "0 9 * * *"
	} else if err := validateCron(body.ScheduleCron); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rt, err := s.db.CreateRoutine(uid, body.Name, body.Prompt, body.ScheduleCron, enabled)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rt)
}

func (s *Server) handlePatchRoutine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	upd := db.RoutineUpdate{}
	if raw, ok := body["name"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.Name = &v
	}
	if raw, ok := body["prompt"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.Prompt = &v
	}
	if raw, ok := body["schedule_cron"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		if err := validateCron(v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		upd.ScheduleCron = &v
	}
	if raw, ok := body["enabled"]; ok {
		var v bool
		_ = json.Unmarshal(raw, &v)
		upd.Enabled = &v
	}
	rt, err := s.db.UpdateRoutine(uid, id, upd)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (s *Server) handleDeleteRoutine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.db.DeleteRoutine(uid, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunRoutine(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	rt, err := s.db.GetRoutine(uid, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	run, err := s.executeRoutine(r.Context(), rt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	fresh, _ := s.db.GetRoutine(uid, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"routine": fresh,
		"run":     run,
	})
}

func (s *Server) executeRoutine(ctx context.Context, rt *db.Routine) (*db.RoutineRun, error) {
	run, err := s.db.CreateRoutineRun(rt.ID, "running", "")
	if err != nil {
		return nil, err
	}
	title := "Routine: " + rt.Name
	agentID, aerr := s.db.ResolveAgentID(rt.UserID, rt.AgentID)
	if aerr != nil {
		_ = s.db.UpdateRoutineRun(run.ID, "failed", aerr.Error())
		run.Status = "failed"
		run.ResultText = aerr.Error()
		return run, aerr
	}
	res, err := s.runAgentOnce(ctx, rt.UserID, agentID, rt.Prompt, title, "", "")
	now := time.Now().UTC()
	_ = s.db.MarkRoutineRun(rt.ID, now)
	if err != nil {
		_ = s.db.UpdateRoutineRun(run.ID, "failed", err.Error())
		run.Status = "failed"
		run.ResultText = err.Error()
		return run, err
	}
	text := res.Reply
	if strings.TrimSpace(text) == "" {
		text = "(empty reply)"
	}
	_ = s.db.UpdateRoutineRun(run.ID, "ok", text)
	run.Status = "ok"
	run.ResultText = text
	return run, nil
}
