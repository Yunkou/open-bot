package httpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

// Bot online (green dot), independent of bot_presence face.
//
// Host check: Connected in hostHub AND last_seen within db.MachineOfflineAfter (90s).
//
// Which machine (product 2026-10-07):
//  1. Conversation session host (conversations.last_machine_id) when set — that
//     machine only (offline stays dark; no silent fallthrough).
//  2. Else agents.machine_id「优先电脑」when set — that machine only.
//  3. Else any of the user's machines online (OR).
//
// ListAgents / agent-global pushes use (2) then (3). Conversation participants
// pass session host via agentHostOnlineSession.

// machineHostOnline is the pure host check used for Bot green-dot online.
// Mobile devices are never host-online (login-only).
func machineHostOnline(m db.Machine, now time.Time) bool {
	if !db.IsHostEligible(m) {
		return false
	}
	if !m.Connected {
		return false
	}
	if m.LastSeen.IsZero() {
		return false
	}
	return now.Sub(m.LastSeen) <= db.MachineOfflineAfter
}

// agentHostOnline is ListAgents / agent-global: 优先电脑 → 任一在线.
func (s *Server) agentHostOnline(userID string, a *db.Agent) bool {
	return s.agentHostOnlineSession(userID, a, "")
}

// agentHostOnlineSession applies session host → 优先电脑 → 任一在线.
func (s *Server) agentHostOnlineSession(userID string, a *db.Agent, sessionMachineID string) bool {
	if s == nil || a == nil {
		return false
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	if sid := strings.TrimSpace(sessionMachineID); sid != "" {
		return s.machineIDHostOnline(userID, sid)
	}
	if pref := strings.TrimSpace(a.MachineID); pref != "" {
		return s.machineIDHostOnline(userID, pref)
	}
	return s.anyUserMachineOnline(userID)
}

// anyUserMachineOnline is true if any registered machine for the user is Connected + fresh.
func (s *Server) anyUserMachineOnline(userID string) bool {
	userID = strings.TrimSpace(userID)
	if s == nil || s.db == nil || s.hosts == nil || userID == "" {
		return false
	}
	machines, err := s.db.ListMachines(userID)
	if err != nil || len(machines) == 0 {
		return false
	}
	now := db.Now()
	for i := range machines {
		m := machines[i]
		m.Connected = s.hosts.Connected(userID, m.ID)
		if machineHostOnline(m, now) {
			return true
		}
	}
	return false
}

// machineIDHostOnline checks one user_machines row by id.
func (s *Server) machineIDHostOnline(userID, machineID string) bool {
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if s == nil || s.db == nil || s.hosts == nil || userID == "" || machineID == "" {
		return false
	}
	m, err := s.db.GetMachine(userID, machineID)
	if err != nil || m == nil {
		return false
	}
	m.Connected = s.hosts.Connected(userID, m.ID)
	return machineHostOnline(*m, db.Now())
}

func (s *Server) stampAgentOnline(userID string, a *db.Agent) {
	if a == nil {
		return
	}
	a.Online = s.agentHostOnline(userID, a)
	s.seedBotOnlineCache(a.ID, a.Online)
}

// seedBotOnlineCache records the bit a client was just served (ListAgents /
// create / patch / clone) when the agent has no cache entry yet, so a later
// flip (e.g. stale heartbeat) is published even after an API restart.
// Never overwrites an existing entry (that would hide a pending flip from SSE-only clients).
func (s *Server) seedBotOnlineCache(agentID string, online bool) {
	agentID = strings.TrimSpace(agentID)
	if s == nil || agentID == "" {
		return
	}
	cache := s.ensureBotOnlineCache()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, ok := cache.seen[agentID]; ok {
		return
	}
	cache.seen[agentID] = struct{}{}
	cache.last[agentID] = online
}

func (s *Server) stampMemberProfilesOnline(userID string, profiles []db.ChannelMemberProfile) {
	s.stampMemberProfilesOnlineSession(userID, profiles, "")
}

func (s *Server) stampMemberProfilesOnlineSession(userID string, profiles []db.ChannelMemberProfile, sessionMachineID string) {
	if len(profiles) == 0 || s == nil || s.db == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	sessionMachineID = strings.TrimSpace(sessionMachineID)
	for i := range profiles {
		aid := strings.TrimSpace(profiles[i].AgentID)
		if aid == "" {
			profiles[i].Online = false
			continue
		}
		a, err := s.db.GetAgent(userID, aid)
		if err != nil || a == nil {
			profiles[i].Online = false
			continue
		}
		profiles[i].Online = s.agentHostOnlineSession(userID, a, sessionMachineID)
	}
}

func (s *Server) enrichChannelOnline(userID string, ch *db.Channel) {
	if ch == nil {
		return
	}
	if len(ch.MemberProfiles) == 0 && len(ch.Members) > 0 && s.db != nil {
		ch.MemberProfiles = s.db.ChannelMemberProfiles(userID, ch.Members)
	}
	s.stampMemberProfilesOnline(userID, ch.MemberProfiles)
}

func (s *Server) conversationParticipants(userID string, c *db.Conversation) []db.ChannelMemberProfile {
	if s == nil || s.db == nil || c == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	var agentIDs []string
	if chID := strings.TrimSpace(c.ChannelID); chID != "" {
		if ch, err := s.db.GetChannel(userID, chID); err == nil && ch != nil && len(ch.Members) > 0 {
			agentIDs = ch.Members
		}
	}
	if len(agentIDs) == 0 {
		if aid := strings.TrimSpace(c.AgentID); aid != "" {
			agentIDs = []string{aid}
		}
	}
	if len(agentIDs) == 0 {
		return nil
	}
	sessionMid := strings.TrimSpace(c.LastMachineID)
	if sessionMid == "" && s.db != nil {
		if mid, err := s.db.ConversationLastMachineID(userID, c.ID); err == nil {
			sessionMid = mid
			c.LastMachineID = mid
		}
	}
	out := s.db.ChannelMemberProfiles(userID, agentIDs)
	s.stampMemberProfilesOnlineSession(userID, out, sessionMid)
	return out
}

func (s *Server) enrichConversationOnline(userID string, c *db.Conversation) {
	if c == nil {
		return
	}
	c.Participants = s.conversationParticipants(userID, c)
}

// publishBotOnline pushes bot_online on chat WS (agent-global: 优先电脑→任一在线)
// and conversation SSE (per-conversation: session host → 优先电脑 → 任一在线).
func (s *Server) publishBotOnline(userID, agentID string, online bool) {
	if s == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	base := map[string]any{
		"type":       "bot_online",
		"agent_id":   agentID,
		"online":     online,
		"updated_at": updatedAt,
	}
	if s.events != nil && userID != "" {
		s.events.Publish(userID, base)
	}
	if s.convEvents == nil || s.db == nil || userID == "" {
		return
	}
	ids, err := s.db.ListConversationIDsForAgent(userID, agentID)
	if err != nil || len(ids) == 0 {
		return
	}
	var agent *db.Agent
	if a, err := s.db.GetAgent(userID, agentID); err == nil {
		agent = a
	}
	for _, convID := range ids {
		bit := online
		if agent != nil {
			mid, _ := s.db.ConversationLastMachineID(userID, convID)
			bit = s.agentHostOnlineSession(userID, agent, mid)
		}
		evt := map[string]any{
			"type":            "bot_online",
			"conversation_id": convID,
			"agent_id":        agentID,
			"online":          bit,
			"updated_at":      updatedAt,
		}
		if payload, err := json.Marshal(evt); err == nil {
			s.convEvents.Publish(convID, payload)
		}
	}
}

var botOnlineInitMu sync.Mutex

// botOnlineCache remembers the last published online bit per agent_id so connect /
// disconnect / heartbeat only emit when that agent's boolean flips.
type botOnlineCache struct {
	mu   sync.Mutex
	last map[string]bool
	seen map[string]struct{}
}

func (s *Server) ensureBotOnlineCache() *botOnlineCache {
	botOnlineInitMu.Lock()
	defer botOnlineInitMu.Unlock()
	if s.botOnline == nil {
		s.botOnline = &botOnlineCache{
			last: make(map[string]bool),
			seen: make(map[string]struct{}),
		}
	}
	return s.botOnline
}

// publishAgentOnlineIfChanged recomputes one agent's host online and emits on flip.
func (s *Server) publishAgentOnlineIfChanged(userID string, a *db.Agent) {
	s.publishAgentOnlineFlip(userID, a, false)
}

// publishAgentOnlineFlip emits bot_online when the agent's bit differs from the
// cache. staleSweep=true (periodic scan of Connected sockets) also emits offline
// for an unseen agent: its socket is Connected, so a client may have rendered
// it online before the heartbeat went stale.
func (s *Server) publishAgentOnlineFlip(userID string, a *db.Agent, staleSweep bool) {
	if s == nil || a == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	agentID := strings.TrimSpace(a.ID)
	if userID == "" || agentID == "" {
		return
	}
	online := s.agentHostOnline(userID, a)
	cache := s.ensureBotOnlineCache()
	cache.mu.Lock()
	_, seen := cache.seen[agentID]
	prev := cache.last[agentID]
	if seen && prev == online {
		cache.mu.Unlock()
		return
	}
	// First observation while offline: seed cache, skip fan-out (avoid spam).
	if !seen && !online && !staleSweep {
		cache.seen[agentID] = struct{}{}
		cache.last[agentID] = false
		cache.mu.Unlock()
		return
	}
	cache.seen[agentID] = struct{}{}
	cache.last[agentID] = online
	cache.mu.Unlock()
	s.publishBotOnline(userID, agentID, online)
}

// publishAgentsOnlineForMachine refreshes bots affected when this host flips:
// preferred binding, session-host conversations, and unbound (任一在线) bots.
func (s *Server) publishAgentsOnlineForMachine(userID, machineID string) {
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if s == nil || s.db == nil || userID == "" || machineID == "" {
		return
	}
	seen := map[string]struct{}{}
	refresh := func(a *db.Agent) {
		if a == nil {
			return
		}
		id := strings.TrimSpace(a.ID)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		s.publishAgentOnlineIfChanged(userID, a)
	}
	if agents, err := s.db.ListAgentsByMachineID(userID, machineID); err == nil {
		for _, a := range agents {
			refresh(a)
		}
	}
	if ids, err := s.db.ListAgentIDsByConversationLastMachine(userID, machineID); err == nil {
		for _, aid := range ids {
			if a, err := s.db.GetAgent(userID, aid); err == nil && a != nil {
				// Session-aware bit for this host (even if preferred differs).
				s.publishAgentOnlineSessionFlip(userID, a, machineID, false)
			}
		}
	}
	if unbound, err := s.db.ListAgentsWithEmptyMachineID(userID); err == nil {
		for _, a := range unbound {
			refresh(a)
		}
	}
}

// publishAgentOnlineSessionFlip publishes using sessionMachineID resolution.
func (s *Server) publishAgentOnlineSessionFlip(userID string, a *db.Agent, sessionMachineID string, staleSweep bool) {
	if s == nil || a == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	agentID := strings.TrimSpace(a.ID)
	if userID == "" || agentID == "" {
		return
	}
	online := s.agentHostOnlineSession(userID, a, sessionMachineID)
	cache := s.ensureBotOnlineCache()
	cache.mu.Lock()
	_, seen := cache.seen[agentID]
	prev := cache.last[agentID]
	if seen && prev == online {
		cache.mu.Unlock()
		return
	}
	if !seen && !online && !staleSweep {
		cache.seen[agentID] = struct{}{}
		cache.last[agentID] = false
		cache.mu.Unlock()
		return
	}
	cache.seen[agentID] = struct{}{}
	cache.last[agentID] = online
	cache.mu.Unlock()
	s.publishBotOnline(userID, agentID, online)
}

// botOnlineSweepInterval: how often Connected host sockets are re-checked
// against last_seen. A frozen host keeps its WS open (pings are answered by the
// OS/runtime) but stops HTTP heartbeats; once last_seen > db.MachineOfflineAfter
// (90s) its bound bots must go dark without a ListAgents refresh. Worst-case
// push latency ≈ 90s + interval.
const botOnlineSweepInterval = 15 * time.Second

// StartBotOnlineSweeper runs sweepBotOnline every botOnlineSweepInterval until ctx is done.
func (s *Server) StartBotOnlineSweeper(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		t := time.NewTicker(botOnlineSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweepBotOnline()
			}
		}
	}()
}

// sweepBotOnline re-evaluates bots bound to every Connected host socket and
// publishes bot_online on flip: Connected ∧ last_seen stale → offline;
// heartbeat resumed while still Connected → online. Disconnects are already
// pushed by hostHub.onChange; machines without a socket are offline regardless
// of last_seen and need no sweep.
func (s *Server) sweepBotOnline() {
	if s == nil || s.db == nil || s.hosts == nil {
		return
	}
	unboundDone := map[string]struct{}{}
	for _, k := range s.hosts.connectedKeys() {
		agents, err := s.db.ListAgentsByMachineID(k.userID, k.machineID)
		if err == nil {
			for _, a := range agents {
				s.publishAgentOnlineFlip(k.userID, a, true)
			}
		}
		if ids, err := s.db.ListAgentIDsByConversationLastMachine(k.userID, k.machineID); err == nil {
			for _, aid := range ids {
				if a, err := s.db.GetAgent(k.userID, aid); err == nil && a != nil {
					s.publishAgentOnlineSessionFlip(k.userID, a, k.machineID, true)
				}
			}
		}
		if _, ok := unboundDone[k.userID]; !ok {
			unboundDone[k.userID] = struct{}{}
			if unbound, err := s.db.ListAgentsWithEmptyMachineID(k.userID); err == nil {
				for _, a := range unbound {
					s.publishAgentOnlineFlip(k.userID, a, true)
				}
			}
		}
	}
}
