package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	ChannelID string    `json:"channel_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// LastMachineID is the desktop host for the latest user message (session host).
	// Empty for browser-only / never-desktop sessions. Not required on every SELECT.
	LastMachineID string    `json:"last_machine_id,omitempty"`
	Messages      []Message `json:"messages,omitempty"`
	// Participants carries agent avatar + online for the primary bot (DM) or channel members.
	// Online is stamped by the API; not stored.
	Participants []ChannelMemberProfile `json:"participants,omitempty"`
}

// MessageSource is a clickable citation URL shown under a Bot reply.
type MessageSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type Message struct {
	ID             string              `json:"id"`
	ConversationID string              `json:"conversation_id,omitempty"`
	Role           string              `json:"role"`
	Content        string              `json:"content"`
	AgentID        string              `json:"agent_id,omitempty"`
	ReplyToID      string              `json:"reply_to_id,omitempty"`
	ThreadRootID   string              `json:"thread_root_id,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	Reactions      []ReactionSummary   `json:"reactions"`
	Attachments    []MessageAttachment `json:"attachments,omitempty"`
	AgentMessageID string              `json:"agent_message_id,omitempty"`
	RequestID      string              `json:"request_id,omitempty"`
	Sources        []MessageSource     `json:"sources,omitempty"`
	Handoff        *HandoffPayload     `json:"handoff,omitempty"`
}

func NormalizeMessageSources(in []MessageSource) []MessageSource {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]MessageSource, 0, len(in))
	for _, s := range in {
		url := strings.TrimSpace(s.URL)
		if url == "" {
			continue
		}
		lower := strings.ToLower(url)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			continue
		}
		key := strings.TrimRight(lower, "/")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item := MessageSource{URL: url}
		if t := strings.TrimSpace(s.Title); t != "" {
			if len(t) > 200 {
				t = t[:200]
			}
			item.Title = t
		}
		out = append(out, item)
		if len(out) >= 20 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var messageSourceURLRe = regexp.MustCompile(`(?i)https?://[^\s<>"')\]]+`)
var messageSourceHostRe = regexp.MustCompile(`(?i)\b(?:[a-z0-9-]+\.)+[a-z]{2,}\b`)

func normalizeSourceURLKey(url string) string {
	u := strings.TrimSpace(url)
	u = strings.TrimRight(u, `/.,;:)]}>"'`)
	return strings.TrimRight(strings.ToLower(u), "/")
}

func citeMatchKeys(url string) map[string]struct{} {
	keys := make(map[string]struct{})
	raw := strings.TrimSpace(url)
	if raw == "" {
		return keys
	}
	full := normalizeSourceURLKey(raw)
	if full != "" {
		keys[full] = struct{}{}
	}
	// Drop query for loose match (answers often omit tracking params).
	if i := strings.Index(full, "?"); i >= 0 {
		keys[strings.TrimRight(full[:i], "/")] = struct{}{}
	}
	return keys
}

func urlsCitedInAnswer(answer string) map[string]struct{} {
	out := make(map[string]struct{})
	if strings.TrimSpace(answer) == "" {
		return out
	}
	for _, m := range messageSourceURLRe.FindAllString(answer, -1) {
		for k := range citeMatchKeys(m) {
			out[k] = struct{}{}
		}
	}
	return out
}

func sourceHost(url string) string {
	u := strings.TrimSpace(url)
	if u == "" {
		return ""
	}
	// Strip scheme
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.Index(u, "@"); i >= 0 {
		u = u[i+1:]
	}
	host := strings.ToLower(strings.TrimSpace(u))
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimPrefix(host, "www.")
	return host
}

func isNoisyCitationURL(url string) bool {
	raw := strings.ToLower(strings.TrimSpace(url))
	host := sourceHost(raw)
	if host == "" {
		return true
	}
	switch host {
	case "duckduckgo.com", "lite.duckduckgo.com", "google.com", "bing.com",
		"purl.org", "w3.org", "xmlns.com":
		return true
	}
	if strings.HasPrefix(host, "api.") || strings.HasPrefix(host, "search.") || strings.HasSuffix(host, ".w3.org") {
		return true
	}
	path := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		path = raw[i+3:]
	}
	if i := strings.Index(path, "/"); i >= 0 {
		path = path[i:]
	} else {
		path = "/"
	}
	if strings.Contains(path, "/api/") || strings.HasPrefix(path, "/api") {
		return true
	}
	if strings.Contains(path, "/rss") || strings.Contains(path, "/feed") || strings.Contains(path, "/atom") ||
		strings.HasSuffix(path, ".xml") || strings.Contains(path, "outboundfeeds") {
		return true
	}
	return false
}

func hostsMentionedInAnswer(answer string) map[string]struct{} {
	out := make(map[string]struct{})
	if strings.TrimSpace(answer) == "" {
		return out
	}
	for _, m := range messageSourceHostRe.FindAllString(answer, -1) {
		h := strings.ToLower(strings.Trim(m, "."))
		h = strings.TrimPrefix(h, "www.")
		if strings.Contains(h, ".") {
			out[h] = struct{}{}
		}
	}
	return out
}

func hostSoftMatch(sourceHost string, mentioned map[string]struct{}) bool {
	if sourceHost == "" || len(mentioned) == 0 {
		return false
	}
	if _, ok := mentioned[sourceHost]; ok {
		return true
	}
	for m := range mentioned {
		if strings.HasSuffix(sourceHost, "."+m) {
			return true
		}
	}
	return false
}

func answerMentionsTitle(answerL, title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if len(t) < 6 {
		return false
	}
	return strings.Contains(answerL, t)
}

func fallbackRetrievalSources(sources []MessageSource, limit int) []MessageSource {
	if limit <= 0 {
		limit = 8
	}
	var out []MessageSource
	seenHost := make(map[string]struct{})
	seenURL := make(map[string]struct{})
	for _, s := range NormalizeMessageSources(sources) {
		if isNoisyCitationURL(s.URL) {
			continue
		}
		host := sourceHost(s.URL)
		if host == "" {
			continue
		}
		if _, ok := seenHost[host]; ok {
			continue
		}
		key := normalizeSourceURLKey(s.URL)
		if _, ok := seenURL[key]; ok {
			continue
		}
		seenHost[host] = struct{}{}
		seenURL[key] = struct{}{}
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterMessageSourcesCited keeps sources the assistant answer actually used.
// Prefer URLs cited in the answer (or structured citations); also soft-match
// host/title mentioned in the text; if the model paraphrased with no links but
// retrieval sources exist, fall back to a curated readable subset (no API/RSS).
func FilterMessageSourcesCited(sources []MessageSource, answer string, structured []MessageSource) []MessageSource {
	if len(sources) == 0 {
		return nil
	}
	cited := urlsCitedInAnswer(answer)
	for _, s := range structured {
		for k := range citeMatchKeys(s.URL) {
			cited[k] = struct{}{}
		}
	}
	answerL := strings.ToLower(answer)
	mentioned := hostsMentionedInAnswer(answer)
	var out []MessageSource
	seen := make(map[string]struct{})
	appendSrc := func(s MessageSource) {
		key := normalizeSourceURLKey(s.URL)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}

	norm := NormalizeMessageSources(sources)
	for _, s := range norm {
		keys := citeMatchKeys(s.URL)
		hit := false
		for k := range keys {
			if _, ok := cited[k]; ok {
				hit = true
				break
			}
		}
		if hit {
			appendSrc(s)
		}
	}
	// Soft host/title only augments when the answer already has URL cites.
	if len(cited) > 0 && len(out) < 20 && strings.TrimSpace(answerL) != "" {
		for _, s := range norm {
			if isNoisyCitationURL(s.URL) {
				continue
			}
			host := sourceHost(s.URL)
			if hostSoftMatch(host, mentioned) || answerMentionsTitle(answerL, s.Title) {
				appendSrc(s)
			}
		}
	}
	if len(out) > 0 {
		if len(out) > 20 {
			out = out[:20]
		}
		return out
	}
	// No raw URL cites: paraphrase path — soft host/title first, then curated fallback.
	if strings.TrimSpace(answer) != "" && len(cited) == 0 {
		var soft []MessageSource
		softSeen := make(map[string]struct{})
		for _, s := range norm {
			if isNoisyCitationURL(s.URL) {
				continue
			}
			host := sourceHost(s.URL)
			if !hostSoftMatch(host, mentioned) && !answerMentionsTitle(answerL, s.Title) {
				continue
			}
			key := normalizeSourceURLKey(s.URL)
			if _, ok := softSeen[key]; ok {
				continue
			}
			softSeen[key] = struct{}{}
			soft = append(soft, s)
		}
		fbLimit := 8
		if len(soft) > 0 {
			if len(soft) > fbLimit {
				return soft[:fbLimit]
			}
			return soft
		}
		return fallbackRetrievalSources(sources, fbLimit)
	}
	return nil
}

func EncodeMessageSources(in []MessageSource) string {
	norm := NormalizeMessageSources(in)
	if len(norm) == 0 {
		return "[]"
	}
	b, err := json.Marshal(norm)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func DecodeMessageSources(raw string) []MessageSource {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var out []MessageSource
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return NormalizeMessageSources(out)
}

func ParseMessageSourcesAny(v any) []MessageSource {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []MessageSource:
		return NormalizeMessageSources(t)
	case []any:
		out := make([]MessageSource, 0, len(t))
		for _, el := range t {
			m, ok := el.(map[string]any)
			if !ok {
				continue
			}
			url, _ := m["url"].(string)
			title, _ := m["title"].(string)
			out = append(out, MessageSource{URL: url, Title: title})
		}
		return NormalizeMessageSources(out)
	case string:
		return DecodeMessageSources(t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return DecodeMessageSources(string(b))
	}
}

// HandoffPayload is the visible Bot↔Bot projection card (not sent to the model).
type HandoffPayload struct {
	FromBot        string `json:"from_bot"`
	ToBot          string `json:"to_bot"`
	Purpose        string `json:"purpose"`
	Status         string `json:"status"` // running|done|failed|rejected|awaiting_approval
	AgentMessageID string `json:"agent_message_id"`
}

// AddMessageOpts carries optional thread fields when inserting a message.
type AddMessageOpts struct {
	AgentID      string
	ReplyToID    string
	ThreadRootID string
	RequestID    string
	At           time.Time
	Sources      []MessageSource
}

// AgentThreadPreview is sidebar metadata for one bot's primary thread.
type AgentThreadPreview struct {
	ConversationID string    `json:"conversation_id,omitempty"`
	LastMessage    string    `json:"last_message,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

func (d *DB) CreateConversation(userID, agentID, title string) (*Conversation, error) {
	resolved, err := d.ResolveAgentID(userID, agentID)
	if err != nil {
		return nil, err
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新对话"
	}
	now := Now()
	c := &Conversation{
		ID:        uuid.NewString(),
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		c.ID, c.UserID, c.AgentID, c.Title, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (d *DB) ListConversations(userID string, limit int) ([]*Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.SQL.Query(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND (channel_id IS NULL OR channel_id = '')
		 ORDER BY updated_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (d *DB) GetConversation(userID, id string) (*Conversation, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (d *DB) EnsureConversation(userID, id, agentID, title string) (*Conversation, error) {
	c, err := d.GetConversation(userID, id)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	resolved, rerr := d.ResolveAgentID(userID, agentID)
	if rerr != nil {
		return nil, rerr
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "会话 " + id
	}
	now := Now()
	c = &Conversation{
		ID:        id,
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (id) DO NOTHING`,
		c.ID, c.UserID, c.AgentID, c.Title, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return d.GetConversation(userID, id)
}

func (d *DB) TouchConversation(userID, id string) error {
	_, err := d.SQL.Exec(
		`UPDATE conversations SET updated_at = $1 WHERE id = $2 AND user_id = $3`,
		Now(), id, userID,
	)
	return err
}

func (d *DB) AddMessage(conversationID, role, content string) (*Message, error) {
	return d.AddMessageWithOpts(conversationID, role, content, AddMessageOpts{})
}

func (d *DB) AddMessageWithAgent(conversationID, role, content, agentID string) (*Message, error) {
	return d.AddMessageWithOpts(conversationID, role, content, AddMessageOpts{AgentID: agentID})
}

func (d *DB) AddMessageAt(conversationID, role, content string, at time.Time) (*Message, error) {
	return d.AddMessageWithOpts(conversationID, role, content, AddMessageOpts{At: at})
}

func (d *DB) AddMessageWithAgentAt(conversationID, role, content, agentID string, at time.Time) (*Message, error) {
	return d.AddMessageWithOpts(conversationID, role, content, AddMessageOpts{AgentID: agentID, At: at})
}

func (d *DB) AddMessageWithOpts(conversationID, role, content string, opts AddMessageOpts) (*Message, error) {
	at := opts.At
	if at.IsZero() {
		at = Now()
	}
	m := &Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		AgentID:        strings.TrimSpace(opts.AgentID),
		ReplyToID:      strings.TrimSpace(opts.ReplyToID),
		ThreadRootID:   strings.TrimSpace(opts.ThreadRootID),
		RequestID:      strings.TrimSpace(opts.RequestID),
		Sources:        NormalizeMessageSources(opts.Sources),
		CreatedAt:      at,
	}
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var replyAny any
	if m.ReplyToID != "" {
		replyAny = m.ReplyToID
	}
	var rootAny any
	if m.ThreadRootID != "" {
		rootAny = m.ThreadRootID
	}
	sourcesJSON := EncodeMessageSources(m.Sources)
	if _, err := tx.Exec(
		`INSERT INTO messages (id, conversation_id, role, content, agent_id, reply_to_id, thread_root_id, request_id, sources_json, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		m.ID, m.ConversationID, m.Role, m.Content, m.AgentID, replyAny, rootAny, m.RequestID, sourcesJSON, m.CreatedAt,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE conversations SET updated_at = $1 WHERE id = $2`,
		m.CreatedAt, conversationID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

// GetMessage returns one message in a conversation the user owns.
func (d *DB) GetMessage(userID, conversationID, messageID string) (*Message, error) {
	if _, err := d.GetConversation(userID, conversationID); err != nil {
		return nil, err
	}
	row := d.SQL.QueryRow(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at,
		        COALESCE(sources_json, '[]')
		 FROM messages WHERE id = $1 AND conversation_id = $2`,
		messageID, conversationID,
	)
	var m Message
	var sourcesRaw string
	if err := row.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID, &m.CreatedAt, &sourcesRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.Sources = DecodeMessageSources(sourcesRaw)
	return &m, nil
}

// FindAssistantByRequestID returns the newest assistant message for a runtime
// request_id in this conversation, if any. Used so the HTTP finish path can
// reuse a row already projected by the durable harness journal (same request_id)
// instead of inserting a duplicate identical reply.
func (d *DB) FindAssistantByRequestID(conversationID, requestID string) (*Message, error) {
	conversationID = strings.TrimSpace(conversationID)
	requestID = strings.TrimSpace(requestID)
	if conversationID == "" || requestID == "" {
		return nil, ErrNotFound
	}
	row := d.SQL.QueryRow(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at,
		        COALESCE(agent_message_id, ''), COALESCE(request_id, ''),
		        COALESCE(sources_json, '[]')
		 FROM messages
		 WHERE conversation_id = $1 AND request_id = $2 AND role = 'assistant'
		 ORDER BY created_at DESC
		 LIMIT 1`,
		conversationID, requestID,
	)
	var m Message
	var sourcesRaw string
	if err := row.Scan(
		&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID,
		&m.CreatedAt, &m.AgentMessageID, &m.RequestID, &sourcesRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.Sources = DecodeMessageSources(sourcesRaw)
	return &m, nil
}

// ResolveThreadRoot returns the thread_root_id for a new reply to parent.
// If parent is already in a thread, inherit; otherwise parent becomes the root.
// Deprecated for chat send: handleSendMessage no longer auto-assigns thread_root
// from reply_to. Prefer explicit client thread_root_id for sidebar-thread posts.
// Kept for any remaining call sites / tools that still need the old Slack semantics.
func ResolveThreadRoot(parent *Message) string {
	if parent == nil {
		return ""
	}
	if r := strings.TrimSpace(parent.ThreadRootID); r != "" {
		return r
	}
	return parent.ID
}

func (d *DB) ListMessages(userID, conversationID string) ([]Message, error) {
	// ownership check
	if _, err := d.GetConversation(userID, conversationID); err != nil {
		return nil, err
	}
	rows, err := d.SQL.Query(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at,
		        COALESCE(agent_message_id, ''), COALESCE(request_id, ''),
		        COALESCE(sources_json, '[]')
		 FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC, id ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var sourcesRaw string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID, &m.CreatedAt, &m.AgentMessageID, &m.RequestID, &sourcesRaw); err != nil {
			return nil, err
		}
		m.Sources = DecodeMessageSources(sourcesRaw)
		if m.Role == "handoff" {
			if hp, herr := ParseHandoffContent(m.Content); herr == nil {
				m.Handoff = hp
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sums, err := d.ListReactionSummariesForConversation(userID, conversationID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if rs, ok := sums[out[i].ID]; ok {
			out[i].Reactions = rs
		} else {
			out[i].Reactions = []ReactionSummary{}
		}
	}
	msgIDs := make([]string, len(out))
	for i := range out {
		msgIDs[i] = out[i].ID
	}
	atts, aerr := d.ListAttachmentsByMessageIDs(userID, conversationID, msgIDs)
	if aerr != nil {
		return nil, aerr
	}
	for i := range out {
		if list, ok := atts[out[i].ID]; ok {
			out[i].Attachments = list
		}
	}
	return out, nil
}

func (d *DB) RecentHostConfirms(conversationID string) ([]Message, error) {
	rows, err := d.SQL.Query(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at
		 FROM messages
		 WHERE conversation_id = $1 AND role = 'host_confirm'
		 ORDER BY created_at DESC
		 LIMIT 40`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMessage removes one message row (e.g. journal-projected PASS cleaned up
// after the HTTP path classifies the turn as group silence).
func (d *DB) DeleteMessage(conversationID, messageID string) error {
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	if conversationID == "" || messageID == "" {
		return ErrNotFound
	}
	res, err := d.SQL.Exec(
		`DELETE FROM messages WHERE id = $1 AND conversation_id = $2`,
		messageID, conversationID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMessageSources replaces citation sources on an existing message (e.g. after
// harness projection, when the HTTP finish path learns retrieval URLs).
func (d *DB) SetMessageSources(conversationID, messageID string, sources []MessageSource) error {
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	if conversationID == "" || messageID == "" {
		return errors.New("conversation_id and message_id required")
	}
	raw := EncodeMessageSources(sources)
	res, err := d.SQL.Exec(
		`UPDATE messages SET sources_json = $1 WHERE id = $2 AND conversation_id = $3`,
		raw, messageID, conversationID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) UpdateMessageContent(conversationID, messageID, content string) (*Message, error) {
	res, err := d.SQL.Exec(
		`UPDATE messages SET content = $1 WHERE id = $2 AND conversation_id = $3`,
		content, messageID, conversationID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	row := d.SQL.QueryRow(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at
		 FROM messages WHERE id = $1`,
		messageID,
	)
	var m Message
	if err := row.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *DB) DeleteConversation(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountUserMessages counts user-role messages in a conversation (no ownership check).
func (d *DB) CountUserMessages(conversationID string) (int, error) {
	var n int
	err := d.SQL.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND role = 'user'`,
		conversationID,
	).Scan(&n)
	return n, err
}

func (d *DB) UpdateConversationTitle(userID, id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title required")
	}
	res, err := d.SQL.Exec(
		`UPDATE conversations SET title = $1, updated_at = $2 WHERE id = $3 AND user_id = $4`,
		title, Now(), id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureChannelConversation returns the conversation bound to a channel (1:1), creating if needed.
func (d *DB) EnsureChannelConversation(userID, channelID, agentID, title string) (*Conversation, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("channel_id required")
	}
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND channel_id = $2`,
		userID, channelID,
	)
	var c Conversation
	err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt)
	if err == nil {
		// Keep agent_id fresh when first member changes.
		agentID = strings.TrimSpace(agentID)
		if agentID != "" && agentID != c.AgentID {
			_, _ = d.SQL.Exec(`UPDATE conversations SET agent_id = $1, updated_at = $2 WHERE id = $3`, agentID, Now(), c.ID)
			c.AgentID = agentID
		}
		return &c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	resolved, rerr := d.ResolveAgentID(userID, agentID)
	if rerr != nil {
		return nil, rerr
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "群聊"
	}
	now := Now()
	c = Conversation{
		ID:        uuid.NewString(),
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		ChannelID: channelID,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, channel_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.UserID, c.AgentID, c.Title, c.ChannelID, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetConversationByChannel looks up the linked conversation without creating.
func (d *DB) GetConversationByChannel(userID, channelID string) (*Conversation, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND channel_id = $2`,
		userID, channelID,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// GetOrCreatePrimaryConversation returns the latest non-channel conversation for (user, agent), creating one if missing.
func (d *DB) GetOrCreatePrimaryConversation(userID, agentID string) (*Conversation, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent id required")
	}
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations
		 WHERE user_id = $1 AND agent_id = $2 AND (channel_id IS NULL OR channel_id = '')
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		userID, agentID,
	)
	var c Conversation
	err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt)
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	title := "与助手的对话"
	return d.CreateConversation(userID, agentID, title)
}

// GetAgentThreadPreview returns last message snippet for the primary thread of an agent.
func (d *DB) GetAgentThreadPreview(userID, agentID string) (*AgentThreadPreview, error) {
	agentID = strings.TrimSpace(agentID)
	row := d.SQL.QueryRow(
		`SELECT c.id, c.updated_at,
		        COALESCE((
		          SELECT CASE
		            WHEN length(m.content) > 80 THEN substr(m.content, 1, 80) || '…'
		            ELSE m.content
		          END
		          FROM messages m
		          WHERE m.conversation_id = c.id AND m.role IN ('user','assistant')
		          ORDER BY m.created_at DESC
		          LIMIT 1
		        ), '')
		 FROM conversations c
		 WHERE c.user_id = $1 AND c.agent_id = $2 AND (c.channel_id IS NULL OR c.channel_id = '')
		 ORDER BY c.updated_at DESC
		 LIMIT 1`,
		userID, agentID,
	)
	var p AgentThreadPreview
	if err := row.Scan(&p.ConversationID, &p.UpdatedAt, &p.LastMessage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &AgentThreadPreview{}, nil
		}
		return nil, err
	}
	return &p, nil
}

func ParseHandoffContent(content string) (*HandoffPayload, error) {
	var hp HandoffPayload
	if err := json.Unmarshal([]byte(content), &hp); err != nil {
		return nil, err
	}
	return &hp, nil
}

func FormatHandoffContent(hp HandoffPayload) (string, error) {
	b, err := json.Marshal(hp)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AddHandoffNote inserts a visible handoff side-note. Idempotent on agent_message_id.
func (d *DB) AddHandoffNote(conversationID, threadRootID string, hp HandoffPayload) (*Message, bool, error) {
	hp.FromBot = strings.TrimSpace(hp.FromBot)
	hp.ToBot = strings.TrimSpace(hp.ToBot)
	hp.Purpose = strings.TrimSpace(hp.Purpose)
	hp.Status = strings.TrimSpace(hp.Status)
	hp.AgentMessageID = strings.TrimSpace(hp.AgentMessageID)
	if hp.FromBot == "" || hp.ToBot == "" || hp.Purpose == "" || hp.Status == "" || hp.AgentMessageID == "" {
		return nil, false, errors.New("from_bot, to_bot, purpose, status, agent_message_id required")
	}
	threadRootID = strings.TrimSpace(threadRootID)

	row := d.SQL.QueryRow(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at,
		        COALESCE(agent_message_id, '')
		 FROM messages WHERE agent_message_id = $1`,
		hp.AgentMessageID,
	)
	var existing Message
	err := row.Scan(&existing.ID, &existing.ConversationID, &existing.Role, &existing.Content, &existing.AgentID, &existing.ReplyToID, &existing.ThreadRootID, &existing.CreatedAt, &existing.AgentMessageID)
	if err == nil {
		existing.Handoff = &hp
		if parsed, perr := ParseHandoffContent(existing.Content); perr == nil {
			existing.Handoff = parsed
		}
		existing.Reactions = []ReactionSummary{}
		return &existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	content, err := FormatHandoffContent(hp)
	if err != nil {
		return nil, false, err
	}
	m := &Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           "handoff",
		Content:        content,
		CreatedAt:      Now(),
		AgentMessageID: hp.AgentMessageID,
		ThreadRootID:   threadRootID,
		Handoff:        &hp,
		Reactions:      []ReactionSummary{},
	}
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var threadArg any
	if threadRootID != "" {
		threadArg = threadRootID
	}
	if _, err := tx.Exec(
		`INSERT INTO messages (id, conversation_id, role, content, created_at, agent_message_id, thread_root_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		m.ID, m.ConversationID, m.Role, m.Content, m.CreatedAt, m.AgentMessageID, threadArg,
	); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(
		`UPDATE conversations SET updated_at = $1 WHERE id = $2`,
		m.CreatedAt, conversationID,
	); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return m, true, nil
}

// ListConversationIDsForAgent returns conversation ids where agent_id is the primary bot
// (DM thread or channel conversation whose agent_id points at this agent).
func (d *DB) ListConversationIDsForAgent(userID, agentID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	agentID = strings.TrimSpace(agentID)
	if userID == "" || agentID == "" {
		return nil, nil
	}
	rows, err := d.SQL.Query(
		`SELECT id FROM conversations WHERE user_id = $1 AND agent_id = $2 ORDER BY updated_at DESC`,
		userID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FindConversationForUserAgent picks the most recently updated conversation for user+agent.
func (d *DB) FindConversationForUserAgent(userID, agentID string) (*Conversation, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = "open-bot"
	}
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND agent_id = $2
		 ORDER BY updated_at DESC LIMIT 1`,
		userID, agentID,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// HasHandoffForAgentMessage reports whether a projection already exists.
func (d *DB) HasHandoffForAgentMessage(agentMessageID string) (bool, error) {
	agentMessageID = strings.TrimSpace(agentMessageID)
	if agentMessageID == "" {
		return false, nil
	}
	var exists bool
	err := d.SQL.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM messages WHERE agent_message_id = $1)`,
		agentMessageID,
	).Scan(&exists)
	return exists, err
}

// UpdateHandoffStatus rewrites status inside an existing handoff note (by agent_message_id).
func (d *DB) UpdateHandoffStatus(agentMessageID, status string) (*Message, error) {
	agentMessageID = strings.TrimSpace(agentMessageID)
	status = strings.TrimSpace(status)
	row := d.SQL.QueryRow(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''),
		        COALESCE(reply_to_id,''), COALESCE(thread_root_id,''), created_at,
		        COALESCE(agent_message_id, '')
		 FROM messages WHERE agent_message_id = $1 AND role = 'handoff'`,
		agentMessageID,
	)
	var m Message
	if err := row.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.ReplyToID, &m.ThreadRootID, &m.CreatedAt, &m.AgentMessageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	hp, err := ParseHandoffContent(m.Content)
	if err != nil {
		return nil, err
	}
	hp.Status = status
	content, err := FormatHandoffContent(*hp)
	if err != nil {
		return nil, err
	}
	if _, err := d.SQL.Exec(`UPDATE messages SET content = $1 WHERE id = $2`, content, m.ID); err != nil {
		return nil, err
	}
	m.Content = content
	m.Handoff = hp
	m.Reactions = []ReactionSummary{}
	return &m, nil
}
