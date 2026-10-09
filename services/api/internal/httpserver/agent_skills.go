package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleListAgentSkills(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	list, err := s.db.ListAgentSkills(uid, agentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": list, "agent_id": agentID})
}

func (s *Server) handleSetAgentSkill(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	name := r.PathValue("name")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sk, err := s.db.SetAgentSkillEnabled(uid, agentID, name, body.Enabled)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleReplaceAgentSkills(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	var body struct {
		Enabled []string `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := s.db.ReplaceAgentSkills(uid, agentID, body.Enabled); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	list, _ := s.db.ListAgentSkills(uid, agentID)
	writeJSON(w, http.StatusOK, map[string]any{"skills": list, "agent_id": agentID})
}

// Onboarding focus presets aligned with BotOnboardingCard (Grok-style job descriptions).
var onboardingPresets = map[string]struct {
	Description  string
	SystemPrompt string
	Skills       []string
}{
	"A": {
		Description:  "负责日常事务与提醒：日程、待办、定时提醒。保持简洁可执行；未经确认不代发外部消息。",
		SystemPrompt: "你主攻日常事务与提醒（日程、待办、定时提醒）。优先给出可执行的下一步；涉及外发或删除需先征得用户同意。",
		Skills:       []string{"daily-brief"},
	},
	"B": {
		Description:  "负责查资料与总结：搜索、阅读、整理要点。输出需可核对；重要结论注明来源或依据。",
		SystemPrompt: "你主攻查资料与总结。先澄清问题边界，再整理要点；区分事实与推测。",
		Skills:       []string{"summarize-text"},
	},
	"C": {
		Description:  "负责写东西与改稿：邮件、文档、文案。尊重用户语气；定稿外发前先给草稿。",
		SystemPrompt: "你主攻写作与改稿。先确认受众与用途，再出草稿；不擅自代发。",
		Skills:       nil, // all user-enabled until customized
	},
	"D": {
		Description:  "负责写代码与排障：改项目、查 bug、联调。小步验证；危险操作需确认。远程 SSH 先 load_skill host-ssh。",
		SystemPrompt: "你主攻写代码与排障。优先小步修改与可验证结果；本机项目用 host_*。复杂流程先 load_skill：排障 diagnosing-bugs、测试先行 tdd、按规格落地 implement、审 diff 用 code-review；入口说明见 dev-assist。远程先 load_skill host-ssh 再 host_ssh_*。",
		Skills: []string{
			"dev-assist",
			"diagnosing-bugs",
			"tdd",
			"implement",
			"code-review",
			"host-ssh",
			"summarize-text",
		},
	},
	"E": {
		Description:  "通用助手：按用户当下话题协助，保持灵活。",
		SystemPrompt: "你是通用助手，先理解用户意图再行动，不强行套用固定流程。",
		Skills:       nil,
	},
}

func (s *Server) handleAgentOnboarding(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.PathValue("id")
	var body struct {
		Focus        string   `json:"focus"` // A–E
		Description  string   `json:"description"`
		SystemPrompt string   `json:"system_prompt"`
		Skills       []string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	focus := strings.ToUpper(strings.TrimSpace(body.Focus))
	desc := strings.TrimSpace(body.Description)
	sp := strings.TrimSpace(body.SystemPrompt)
	var skills []string
	skillsSet := false
	if body.Skills != nil {
		skills = body.Skills
		skillsSet = true
	}
	if preset, ok := onboardingPresets[focus]; ok {
		if desc == "" {
			desc = preset.Description
		}
		if sp == "" {
			sp = preset.SystemPrompt
		}
		if !skillsSet && preset.Skills != nil {
			skills = preset.Skills
			skillsSet = true
		}
	}
	var skillArg []string
	if skillsSet {
		skillArg = skills
		if skillArg == nil {
			skillArg = []string{}
		}
	} else {
		skillArg = nil // leave agent_skills unset → all user-enabled
	}
	a, err := s.db.ApplyAgentOnboarding(uid, agentID, desc, sp, skillArg)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": a, "focus": focus})
}
