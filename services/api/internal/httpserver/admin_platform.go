package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handlePlatformListOrgs(w http.ResponseWriter, r *http.Request) {
	days := 30
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			days = n
		}
	}
	summaries, err := s.db.PlatformOrgSummaries(days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orgs":  summaries,
		"days":  days,
		"count": len(summaries),
	})
}

func (s *Server) handlePlatformOrgUsage(w http.ResponseWriter, r *http.Request) {
	orgID := strings.TrimSpace(r.PathValue("id"))
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "org id required"})
		return
	}
	if _, err := s.db.GetOrgByID(orgID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "org not found"})
		return
	}
	days := 30
	if q := strings.TrimSpace(r.URL.Query().Get("days")); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			days = n
		}
	}
	usage, err := s.db.GetOrgUsageDetailed(orgID, days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// handlePlatformCreateOrg creates a tenant (platform_admin only).
func (s *Server) handlePlatformCreateOrg(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	slug := strings.ToLower(strings.TrimSpace(body.Slug))
	name := strings.TrimSpace(body.Name)
	if slug == "" || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and name required"})
		return
	}
	org, err := s.db.CreateOrg(slug, name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(u.OrgID, u.ID, "platform.org_create", "org", org.ID, map[string]any{
		"slug": org.Slug, "name": org.Name,
	})
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) handleAdminMeScope(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	scopeOrg := s.adminScopeOrgID(r, u)
	org, _ := s.db.GetOrgByID(scopeOrg)
	out := map[string]any{
		"me":              u.PublicMap(),
		"home_org_id":     u.OrgID,
		"scope_org_id":    scopeOrg,
		"is_platform":     db.NormalizeRole(u.Role) == db.RolePlatformAdmin,
		"can_switch_org":  db.NormalizeRole(u.Role) == db.RolePlatformAdmin,
	}
	if org != nil {
		out["scope_org"] = org
	}
	writeJSON(w, http.StatusOK, out)
}
