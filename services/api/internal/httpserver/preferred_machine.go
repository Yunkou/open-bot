package httpserver

import (
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

// attachPreferredMachine stamps agents.machine_id as preferred_machine_id for
// the runtime host router (optional「优先电脑」). Empty when unbound.
// Mobile preferred bindings are omitted so runtime never routes host ops to phones.
func (s *Server) attachPreferredMachine(payload map[string]any, userID, agentID string) {
	if s == nil || payload == nil || s.db == nil {
		return
	}
	a, err := s.db.GetAgent(userID, agentID)
	if err != nil || a == nil {
		return
	}
	mid := strings.TrimSpace(a.MachineID)
	if mid == "" {
		return
	}
	m, err := s.db.GetMachine(userID, mid)
	if err != nil || m == nil || !db.IsHostEligible(*m) {
		return
	}
	payload["preferred_machine_id"] = mid
}
