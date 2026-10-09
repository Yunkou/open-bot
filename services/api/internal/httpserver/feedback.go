package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

type feedbackBody struct {
	MessageID      string   `json:"message_id"`
	AgentID        string   `json:"agent_id"`
	ConversationID string   `json:"conversation_id"`
	Polarity       string   `json:"polarity"`
	Reasons        []string `json:"reasons"`
	Note           string   `json:"note"`
	Source         string   `json:"source"`
}

func (s *Server) handleCreateFeedback(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body feedbackBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	f, err := s.db.CreateMessageFeedback(uid, db.MessageFeedback{
		MessageID: body.MessageID, AgentID: body.AgentID, ConversationID: body.ConversationID,
		Polarity: body.Polarity, Reasons: body.Reasons, Note: body.Note, Source: body.Source,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// Experience draft (pending): does NOT inject until user confirms → active.
	_, _ = s.draftLessonFromFeedback(uid, f)
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.db.ListMessageFeedbacks(uid, agentID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.MessageFeedback{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedbacks": list})
}

type lessonBody struct {
	FeedbackID string   `json:"feedback_id"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Tags       []string `json:"tags"`
	Status     string   `json:"status"`
}

func (s *Server) handleCreateLesson(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	var body lessonBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	l, err := s.db.CreateBotLesson(uid, db.BotLesson{
		AgentID: agentID, FeedbackID: body.FeedbackID, Title: body.Title, Body: body.Body, Tags: body.Tags, Status: body.Status,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleListLessons(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.db.ListBotLessons(uid, agentID, status, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.BotLesson{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lessons": list})
}

func (s *Server) handleListActiveLessons(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	list, err := s.db.ListActiveBotLessons(uid, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.BotLesson{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lessons": list})
}

type lessonPatchBody struct {
	Title  *string   `json:"title"`
	Body   *string   `json:"body"`
	Tags   *[]string `json:"tags"`
	Status *string   `json:"status"`
}

func (s *Server) handlePatchLesson(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	var body lessonPatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	l, err := s.db.UpdateBotLesson(uid, id, db.BotLessonPatch{
		Title: body.Title, Body: body.Body, Tags: body.Tags, Status: body.Status,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleDeleteLesson(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.db.DeleteBotLesson(uid, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Internal: runtime pulls active lessons for injection.
func (s *Server) handleInternalActiveLessons(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	if userID == "" || agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and agent_id required"})
		return
	}
	list, err := s.db.ListActiveBotLessons(userID, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.BotLesson{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lessons": list})
}

// draftLessonFromFeedback builds a pending lesson from feedback; never status=active.
func (s *Server) draftLessonFromFeedback(userID string, f *db.MessageFeedback) (*db.BotLesson, error) {
	if f == nil {
		return nil, nil
	}
	title, body, tags := lessonDraftFromFeedback(f)
	if title == "" || body == "" {
		return nil, nil
	}
	return s.db.CreateBotLesson(userID, db.BotLesson{
		AgentID:    f.AgentID,
		FeedbackID: f.ID,
		Title:      title,
		Body:       body,
		Tags:       tags,
		Status:     "pending",
	})
}

func lessonDraftFromFeedback(f *db.MessageFeedback) (title, body string, tags []string) {
	tags = append([]string{}, f.Reasons...)
	if tags == nil {
		tags = []string{}
	}
	note := strings.TrimSpace(f.Note)
	reasonText := feedbackReasonText(f.Reasons)
	if f.Polarity == "negative" {
		title = "避免重复同类问题"
		if reasonText != "" {
			title = "改进：" + lessonClip(reasonText, 16)
		}
		body = "用户对本条回复给出了负面反馈。"
		if reasonText != "" {
			body += "原因：" + reasonText + "。"
		}
		if note != "" {
			body += "用户希望：" + note
		} else {
			body += "下次回答时针对上述原因修正，先对齐要求再展开。"
		}
	} else {
		title = "保持有效做法"
		if reasonText != "" {
			title = "强化：" + lessonClip(reasonText, 16)
		}
		body = "用户对本条回复给出了正面反馈。"
		if reasonText != "" {
			body += "认可点：" + reasonText + "。"
		}
		if note != "" {
			body += "补充：" + note
		} else {
			body += "同类问题可沿用当前结构与语气。"
		}
	}
	title = lessonClip(title, 40)
	body = lessonClip(body, 500)
	return title, body, tags
}

// lessonClip trims to max runes without an ellipsis (lesson title/body limits are rune counts).
func lessonClip(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// feedbackReasonLabels maps web FeedbackModal reason ids to readable Chinese labels.
var feedbackReasonLabels = map[string]string{
	"wrong":        "答错了/事实不准",
	"verbose":      "太啰嗦",
	"not_followed": "没按要求做",
	"missed_steps": "漏了关键步骤",
	"tone":         "语气/格式不对",
	"other":        "其它",
	"good":         "答得好",
	"concise":      "简洁有用",
	"format_ok":    "格式对",
}

func feedbackReasonText(reasons []string) string {
	out := make([]string, 0, len(reasons))
	for _, r := range reasons {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if l, ok := feedbackReasonLabels[r]; ok {
			out = append(out, l)
		} else {
			out = append(out, r)
		}
	}
	return strings.Join(out, "、")
}
