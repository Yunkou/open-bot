package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

type runOnceResult struct {
	ConversationID string
	Reply          string
	Summary        string
}

// runAgentOnce creates a conversation (or reuses convID), appends user message,
// calls runtime SSE, persists assistant reply, and returns collected text.
// extraSystem is appended to the agent system_prompt when non-empty.
func (s *Server) runAgentOnce(ctx context.Context, userID, agentID, content, title, convID, extraSystem string) (*runOnceResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("content required")
	}
	resolved, rerr := s.db.ResolveAgentID(userID, agentID)
	if rerr != nil {
		return nil, rerr
	}
	agentID = resolved

	var conv *db.Conversation
	var err error
	if strings.TrimSpace(convID) != "" {
		conv, err = s.db.EnsureConversation(userID, convID, agentID, title)
	} else {
		if strings.TrimSpace(title) == "" {
			title = "例行任务"
		}
		conv, err = s.db.CreateConversation(userID, agentID, title)
	}
	if err != nil {
		return nil, err
	}

	userMsg, err := s.db.AddMessage(conv.ID, "user", content)
	if err != nil {
		return nil, err
	}

	msgs, err := s.db.ListMessages(userID, conv.ID)
	if err != nil {
		return nil, err
	}
	history := historyForRuntime(msgs)

	var llmPayload map[string]any
	conn, err := s.db.ResolveEffectiveLLM(userID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	if conn != nil {
		llmPayload = llmRuntimePayload(conn)
	}

	systemPrompt := ""
	if agent, aerr := s.db.GetAgent(userID, agentID); aerr == nil {
		systemPrompt = agent.SystemPrompt
	}
	if es := strings.TrimSpace(extraSystem); es != "" {
		if systemPrompt != "" {
			systemPrompt = systemPrompt + "\n\n" + es
		} else {
			systemPrompt = es
		}
	}

	enabledSkills, _ := s.db.ListEnabledSkillNames(userID)
	if enabledSkills == nil {
		enabledSkills = []string{}
	}

	payloadMap := map[string]any{
		"conversation_id": conv.ID,
		"content":         content,
		"agent_id":        agentID,
		"user_id":         userID,
		"system_prompt":   systemPrompt,
		"messages":        history,
		"enabled_skills":  enabledSkills,
	}
	if llmPayload != nil {
		payloadMap["llm"] = llmPayload
	}
	payload, _ := json.Marshal(payloadMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.runtimeURL+"/v1/runs", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("runtime error: %s", strings.TrimSpace(string(b)))
	}

	var assistant strings.Builder
	var eventName string
	var pendingSummary string
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(string(line), "\r\n")
			if strings.HasPrefix(trimmed, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if strings.HasPrefix(trimmed, "data:") {
				raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				var payload map[string]any
				if json.Unmarshal([]byte(raw), &payload) == nil {
					switch eventName {
					case "token":
						if text, ok := payload["text"].(string); ok {
							assistant.WriteString(text)
						}
					case "meta":
						if summaryNew, _ := payload["summary_new"].(bool); summaryNew {
							if sum, ok := payload["summary"].(string); ok && strings.TrimSpace(sum) != "" {
								pendingSummary = sum
							}
						}
					case "error":
						if msg, ok := payload["message"].(string); ok && msg != "" {
							return nil, errors.New(msg)
						}
					}
				}
			} else if trimmed == "" {
				eventName = ""
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
	}

	if pendingSummary != "" {
		sumAt := userMsg.CreatedAt.Add(-time.Millisecond)
		_, _ = s.db.AddMessageAt(conv.ID, "summary", pendingSummary, sumAt)
	}
	reply := assistant.String()
	if reply != "" {
		_, _ = s.db.AddMessage(conv.ID, "assistant", reply)
	}
	return &runOnceResult{
		ConversationID: conv.ID,
		Reply:          reply,
		Summary:        pendingSummary,
	}, nil
}
