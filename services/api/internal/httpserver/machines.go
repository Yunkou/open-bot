package httpserver

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/auth"
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
	s.markConnected(uid, list)
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
	s.markConnected(uid, list)
	slim := make([]map[string]any, 0, len(list))
	for _, m := range list {
		slim = append(slim, map[string]any{
			"id":            m.ID,
			"label":         m.Label,
			"platform":      m.Platform,
			"os":            m.OS,
			"arch":          m.Arch,
			"app":           m.App,
			"status":        m.Status,
			"last_seen":     db.FormatTime(m.LastSeen),
			"online":        m.Connected,
			"connected":     m.Connected,
			"file_op_count": m.FileOpCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": slim, "count": len(slim)})
}

func (s *Server) markConnected(userID string, list []db.Machine) {
	if s.hosts == nil {
		return
	}
	for i := range list {
		list[i].Connected = s.hosts.Connected(userID, list[i].ID)
	}
}

func (s *Server) handleHostExecWS(w http.ResponseWriter, r *http.Request) {
	tok := jwtFromRequest(r)
	claims, err := auth.ParseToken(tok)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	id := r.PathValue("id")
	m, err := s.db.GetMachine(claims.UserID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "machine not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	conn, err := busUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("host exec ws upgrade: %v", err)
		return
	}
	if _, err := s.db.HeartbeatMachine(claims.UserID, m.ID); err != nil {
		log.Printf("host exec heartbeat: %v", err)
	}
	s.hosts.serve(claims.UserID, m.ID, conn)
}

func hostActivityLabel(label, op string) string {
	action := "处理文件"
	switch op {
	case "ls":
		action = "列出目录"
	case "read":
		action = "读取文件"
	case "write":
		action = "写入文件"
	case "delete":
		action = "删除文件"
	case "move":
		action = "移动文件"
	case "open":
		action = "打开软件"
	case "shell":
		action = "运行命令"
	}
	msg := "正在" + label + "上" + action
	if op == "write" || op == "delete" || op == "move" || op == "shell" {
		msg += "。若需要确认，在对话里点允许或拒绝"
	}
	return msg
}

func (s *Server) handleInternalHostExec(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID         string `json:"user_id"`
		MachineID      string `json:"machine_id"`
		ConversationID string `json:"conversation_id"`
		Op             string `json:"op"`
		Path           string `json:"path"`
		Dest           string `json:"dest"`
		Content        string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	mid := strings.TrimSpace(body.MachineID)
	op := strings.TrimSpace(body.Op)
	if uid == "" || mid == "" || op == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, machine_id and op required"})
		return
	}
	switch op {
	case "ls", "read", "write", "delete", "move", "open", "shell":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported op"})
		return
	}
	if op != "ls" && strings.TrimSpace(body.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path required"})
		return
	}
	if op == "shell" && len(body.Path) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command too long"})
		return
	}
	if op == "open" && len(strings.TrimSpace(body.Path)) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "app name too long"})
		return
	}
	if len(body.Content) > 1<<20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content too large"})
		return
	}
	m, err := s.db.GetMachine(uid, mid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "没有这台已登记的电脑"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !s.hosts.Connected(uid, mid) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"connected":  false,
			"machine_id": mid,
			"label":      m.Label,
			"error":      "「" + m.Label + "」的应用没开着，请先在那台电脑上打开",
		})
		return
	}
	if s.events != nil {
		s.events.Publish(uid, map[string]any{
			"type":       "host_activity",
			"active":     true,
			"machine_id": mid,
			"label":      hostActivityLabel(m.Label, op),
		})
	}
	result, err := s.hosts.Call(r.Context(), uid, mid, hostExecRequest{
		Op: op, Path: body.Path, Dest: body.Dest, Content: body.Content,
		ConversationID: strings.TrimSpace(body.ConversationID),
	})
	if s.events != nil {
		s.events.Publish(uid, map[string]any{
			"type":       "host_activity",
			"active":     false,
			"machine_id": mid,
		})
	}
	s.finishHostConfirm(uid, strings.TrimSpace(body.ConversationID), result)
	if err != nil {
		if errors.Is(err, errHostNotConnected) {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "connected": false, "machine_id": mid, "label": m.Label,
				"error": "「" + m.Label + "」的应用没开着，请先在那台电脑上打开",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "machine_id": mid})
		return
	}
	if ok, _ := result["ok"].(bool); ok {
		if _, incErr := s.db.IncrementMachineFileOps(uid, mid); incErr != nil {
			log.Printf("increment file ops: %v", incErr)
		}
		if usual, uErr := s.db.UsualWorkMachine(uid); uErr == nil && usual != nil {
			result["usual"] = map[string]any{
				"id":            usual.ID,
				"label":         usual.Label,
				"platform":      usual.Platform,
				"file_op_count": usual.FileOpCount,
			}
		}
	}
	result["machine_id"] = mid
	result["label"] = m.Label
	writeJSON(w, http.StatusOK, result)
}

type hostConfirmPayload struct {
	ReqID   string `json:"req_id"`
	Op      string `json:"op"`
	Path    string `json:"path"`
	Dest    string `json:"dest,omitempty"`
	Preview string `json:"preview,omitempty"`
	Status  string `json:"status"`
}

func hostConfirmStatus(result map[string]any) string {
	if result == nil {
		return "denied"
	}
	if denied, _ := result["denied"].(bool); denied {
		return "denied"
	}
	if ok, _ := result["ok"].(bool); ok {
		return "allowed"
	}
	if confirmed, _ := result["confirmed"].(bool); confirmed {
		return "allowed"
	}
	return "denied"
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (s *Server) finishHostConfirm(userID, conversationID string, result map[string]any) {
	if s == nil || conversationID == "" || result == nil {
		return
	}
	reqID, _ := result["req_id"].(string)
	reqID = strings.TrimSpace(reqID)
	if reqID == "" {
		return
	}
	msg, err := s.db.RecentHostConfirms(conversationID)
	if err != nil {
		log.Printf("host confirm lookup: %v", err)
		return
	}
	status := hostConfirmStatus(result)
	for i := range msg {
		var payload hostConfirmPayload
		if json.Unmarshal([]byte(msg[i].Content), &payload) != nil || payload.ReqID != reqID {
			continue
		}
		if payload.Status == status {
			return
		}
		payload.Status = status
		raw, mErr := json.Marshal(payload)
		if mErr != nil {
			return
		}
		updated, uErr := s.db.UpdateMessageContent(conversationID, msg[i].ID, string(raw))
		if uErr != nil {
			log.Printf("host confirm update: %v", uErr)
			return
		}
		s.publishConversationMessage(userID, updated)
		return
	}
}

func (s *Server) handleCreateHostConfirm(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	conversationID := r.PathValue("id")
	if _, err := s.db.GetConversation(uid, conversationID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var body hostConfirmPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body.ReqID = strings.TrimSpace(body.ReqID)
	body.Op = strings.TrimSpace(body.Op)
	body.Path = strings.TrimSpace(body.Path)
	body.Dest = strings.TrimSpace(body.Dest)
	body.Preview = clipRunes(body.Preview, 180)
	body.Status = "pending"
	if body.ReqID == "" || body.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "req_id and path required"})
		return
	}
	switch body.Op {
	case "write", "delete", "move", "shell":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported op"})
		return
	}
	existing, err := s.db.RecentHostConfirms(conversationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i := range existing {
		var payload hostConfirmPayload
		if json.Unmarshal([]byte(existing[i].Content), &payload) == nil && payload.ReqID == body.ReqID {
			writeJSON(w, http.StatusOK, existing[i])
			return
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	msg, err := s.db.AddMessage(conversationID, "host_confirm", string(raw))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publishConversationMessage(uid, msg)
	writeJSON(w, http.StatusOK, msg)
}

func (s *Server) handleDecideHostConfirm(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	conversationID := r.PathValue("id")
	messageID := r.PathValue("msgId")
	if _, err := s.db.GetConversation(uid, conversationID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	status := strings.TrimSpace(body.Status)
	if status != "allowed" && status != "denied" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be allowed or denied"})
		return
	}
	found, err := s.db.RecentHostConfirms(conversationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i := range found {
		if found[i].ID != messageID {
			continue
		}
		var payload hostConfirmPayload
		if err := json.Unmarshal([]byte(found[i].Content), &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid confirm payload"})
			return
		}
		if payload.Status == status || (payload.Status != "" && payload.Status != "pending") {
			writeJSON(w, http.StatusOK, found[i])
			return
		}
		payload.Status = status
		raw, mErr := json.Marshal(payload)
		if mErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": mErr.Error()})
			return
		}
		updated, uErr := s.db.UpdateMessageContent(conversationID, messageID, string(raw))
		if uErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": uErr.Error()})
			return
		}
		s.publishConversationMessage(uid, updated)
		writeJSON(w, http.StatusOK, updated)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "confirm not found"})
}
