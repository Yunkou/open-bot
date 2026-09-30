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

func validateCronOptional(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	return validateCron(expr)
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
		Name           string           `json:"name"`
		Prompt         string           `json:"prompt"`
		ScheduleCron   string           `json:"schedule_cron"`
		Enabled        *bool            `json:"enabled"`
		AgentID        string           `json:"agent_id"`
		Timezone       string           `json:"timezone"`
		ConversationID string           `json:"conversation_id"`
		TriggersJSON   string           `json:"triggers_json"`
		Triggers       []db.RoutineTrigger `json:"triggers"`
		MaxRetries     *int             `json:"max_retries"`
		QuietUnchanged *bool            `json:"quiet_unchanged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	triggersJSON := strings.TrimSpace(body.TriggersJSON)
	if triggersJSON == "" && body.Triggers != nil {
		encoded, err := db.EncodeTriggersJSON(body.Triggers)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid triggers"})
			return
		}
		triggersJSON = encoded
	}
	if err := validateCronOptional(body.ScheduleCron); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	maxRetries := 2
	if body.MaxRetries != nil {
		maxRetries = *body.MaxRetries
	}
	quiet := false
	if body.QuietUnchanged != nil {
		quiet = *body.QuietUnchanged
	}
	rt, err := s.db.CreateRoutine(
		uid, body.Name, body.Prompt, body.ScheduleCron, enabled, body.AgentID,
		body.Timezone, body.ConversationID, triggersJSON, maxRetries, quiet,
	)
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
		if err := validateCronOptional(v); err != nil {
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
	if raw, ok := body["agent_id"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.AgentID = &v
	}
	if raw, ok := body["timezone"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.Timezone = &v
	}
	if raw, ok := body["conversation_id"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.ConversationID = &v
	}
	if raw, ok := body["triggers_json"]; ok {
		var v string
		_ = json.Unmarshal(raw, &v)
		upd.TriggersJSON = &v
	}
	if raw, ok := body["triggers"]; ok {
		var tr []db.RoutineTrigger
		if err := json.Unmarshal(raw, &tr); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid triggers"})
			return
		}
		encoded, err := db.EncodeTriggersJSON(tr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid triggers"})
			return
		}
		upd.TriggersJSON = &encoded
	}
	if raw, ok := body["max_retries"]; ok {
		var v int
		_ = json.Unmarshal(raw, &v)
		upd.MaxRetries = &v
	}
	if raw, ok := body["quiet_unchanged"]; ok {
		var v bool
		_ = json.Unmarshal(raw, &v)
		upd.QuietUnchanged = &v
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
	run, err := s.executeRoutine(r.Context(), rt, "")
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

// executeRoutine runs a routine into its pinned conversation (created on first fire).
// eventContext is optional text appended after the prompt (Slack/GitHub payload summary).
func (s *Server) executeRoutine(ctx context.Context, rt *db.Routine, eventContext string) (*db.RoutineRun, error) {
	run, err := s.db.CreateRoutineRun(rt.ID, "running", "")
	if err != nil {
		return nil, err
	}
	agentID, aerr := s.db.ResolveAgentID(rt.UserID, rt.AgentID)
	if aerr != nil {
		_ = s.db.UpdateRoutineRun(run.ID, "failed", aerr.Error())
		_ = s.db.SetRoutineResultMeta(rt.ID, rt.LastResultHash, rt.FailCount+1, aerr.Error())
		run.Status = "failed"
		run.ResultText = aerr.Error()
		return run, aerr
	}

	title := "例行 · " + rt.Name
	convID := strings.TrimSpace(rt.ConversationID)
	if convID == "" {
		conv, cerr := s.db.CreateConversation(rt.UserID, agentID, title)
		if cerr != nil {
			_ = s.db.UpdateRoutineRun(run.ID, "failed", cerr.Error())
			run.Status = "failed"
			run.ResultText = cerr.Error()
			return run, cerr
		}
		convID = conv.ID
		_ = s.db.SetRoutineConversationID(rt.ID, convID)
		rt.ConversationID = convID
	}

	content := "[routine] " + rt.Name + "\n\n" + strings.TrimSpace(rt.Prompt)
	if ec := strings.TrimSpace(eventContext); ec != "" {
		content += "\n\n[event]\n" + ec
	}
	extraSystem := "这是例行任务唤醒。请按 prompt 执行并给出简短中文报告；若 quiet_unchanged 场景下无变化，可回复「无变化」。"

	maxAttempts := 1 + rt.MaxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 6 {
		maxAttempts = 6
	}

	var lastErr error
	var text string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		res, err := s.runAgentOnce(ctx, rt.UserID, agentID, content, title, convID, extraSystem)
		if err != nil {
			lastErr = err
			continue
		}
		text = res.Reply
		if strings.TrimSpace(text) == "" {
			text = "(empty reply)"
		}
		lastErr = nil
		break
	}

	now := time.Now().UTC()
	_ = s.db.MarkRoutineRun(rt.ID, now)

	if lastErr != nil {
		errText := lastErr.Error()
		_ = s.db.UpdateRoutineRun(run.ID, "failed", errText)
		_ = s.db.SetRoutineResultMeta(rt.ID, rt.LastResultHash, rt.FailCount+1, errText)
		run.Status = "failed"
		run.ResultText = errText
		// Still surface failure in the pinned conversation.
		if msg, merr := s.db.AddMessageWithAgent(convID, "assistant", "例行任务失败："+errText, agentID); merr == nil {
			s.publishConversationMessage(rt.UserID, msg)
		}
		return run, lastErr
	}

	hash := db.HashResultText(text)
	quietSkip := rt.QuietUnchanged && rt.LastResultHash != "" && hash == rt.LastResultHash
	status := "ok"
	if quietSkip {
		status = "ok_quiet"
	}
	_ = s.db.UpdateRoutineRun(run.ID, status, text)
	_ = s.db.SetRoutineResultMeta(rt.ID, hash, 0, "")
	run.Status = status
	run.ResultText = text

	if !quietSkip {
		// runAgentOnce already persisted user+assistant into conv; publish for live UI.
		if msgs, err := s.db.ListMessages(rt.UserID, convID); err == nil && len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			if last.Role == "assistant" {
				msg := last
				s.publishConversationMessage(rt.UserID, &msg)
			}
		}
	}
	return run, nil
}
