package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleAdminListSkills(w http.ResponseWriter, r *http.Request) {
	includeDisabled := r.URL.Query().Get("all") == "1"
	list, err := s.db.ListGlobalSkillsFromDB(includeDisabled)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Omit full body in list responses.
	out := make([]map[string]any, 0, len(list))
	for _, sk := range list {
		out = append(out, map[string]any{
			"name":        sk.Name,
			"description": sk.Description,
			"enabled":     sk.Enabled,
			"created_at":  sk.CreatedAt,
			"updated_at":  sk.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

func (s *Server) handleAdminGetSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := s.db.GetGlobalSkill(name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleAdminUpsertSkillFile(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		path = r.PathValue("path")
	}
	sk, err := s.db.UpsertGlobalSkillFile(name, path, body.Content)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.file_upsert", "skill", sk.Name, map[string]any{
		"path": path,
	})
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleAdminDeleteSkillFile(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	path := r.PathValue("path")
	// PathValue may not include slashes; allow ?path= for nested files.
	if q := strings.TrimSpace(r.URL.Query().Get("path")); q != "" {
		path = q
	}
	if err := s.db.DeleteGlobalSkillFile(name, path); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.file_delete", "skill", name, map[string]any{
		"path": path,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminUpsertSkill(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		BodyMarkdown string `json:"body_markdown"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = r.PathValue("name")
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	sk, err := s.db.UpsertGlobalSkill(name, body.Description, body.BodyMarkdown, enabled)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.upsert", "skill", sk.Name, map[string]any{
		"enabled": sk.Enabled,
	})
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleAdminPatchSkill(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body struct {
		Description  *string `json:"description"`
		BodyMarkdown *string `json:"body_markdown"`
		Enabled      *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	existing, err := s.db.GetGlobalSkill(name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	desc := existing.Description
	md := existing.BodyMarkdown
	en := existing.Enabled
	if body.Description != nil {
		desc = *body.Description
	}
	if body.BodyMarkdown != nil {
		md = *body.BodyMarkdown
	}
	if body.Enabled != nil {
		en = *body.Enabled
	}
	sk, err := s.db.UpsertGlobalSkill(name, desc, md, en)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.update", "skill", sk.Name, map[string]any{
		"enabled": sk.Enabled,
	})
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleAdminDeleteSkill(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if err := s.db.DeleteGlobalSkill(name); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.delete", "skill", name, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminImportSkillZip(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart"})
		return
	}
	nameHint := strings.TrimSpace(r.FormValue("name"))
	descHint := strings.TrimSpace(r.FormValue("description"))
	enabled := true
	if v := strings.TrimSpace(r.FormValue("enabled")); v == "0" || strings.EqualFold(v, "false") {
		enabled = false
	}
	file, hdr, err := r.FormFile("archive")
	if err != nil {
		file, hdr, err = r.FormFile("file")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "archive (.zip) required"})
		return
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, 8<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read archive"})
		return
	}
	fn := strings.ToLower(hdr.Filename)
	if !strings.HasSuffix(fn, ".zip") && !(len(b) >= 4 && b[0] == 'P' && b[1] == 'K') {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "archive must be a .zip"})
		return
	}
	parsed, err := db.ParseZipSkillPackage(b)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sk, err := s.db.ImportGlobalSkillPackage(parsed, nameHint, descHint, enabled)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "skill.import_zip", "skill", sk.Name, map[string]any{
		"files": len(sk.Files),
	})
	writeJSON(w, http.StatusOK, sk)
}

func (s *Server) handleAdminExportSkillZip(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, filename, err := s.db.ExportGlobalSkillZip(name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleAdminListUserSkills(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("id")
	target, err := s.db.GetUserByID(userID)
	if err != nil || target.OrgID != admin.OrgID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
		return
	}
	list, err := s.db.ListUserSkills(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": list, "user_id": userID})
}

func (s *Server) handleAdminSetUserSkill(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("id")
	name := r.PathValue("name")
	target, err := s.db.GetUserByID(userID)
	if err != nil || target.OrgID != admin.OrgID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sk, err := s.db.SetUserSkillEnabled(userID, name, body.Enabled)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "user.skill_toggle", "user", userID, map[string]any{
		"skill":   name,
		"enabled": body.Enabled,
	})
	writeJSON(w, http.StatusOK, sk)
}
