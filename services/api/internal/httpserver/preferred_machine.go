package httpserver

import "strings"

// attachPreferredMachine stamps agents.machine_id as preferred_machine_id for
// the runtime host router (optional「优先电脑」). Empty when unbound.
func (s *Server) attachPreferredMachine(payload map[string]any, userID, agentID string) {
	if s == nil || payload == nil || s.db == nil {
		return
	}
	a, err := s.db.GetAgent(userID, agentID)
	if err != nil || a == nil {
		return
	}
	if mid := strings.TrimSpace(a.MachineID); mid != "" {
		payload["preferred_machine_id"] = mid
	}
}
