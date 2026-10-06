package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

// Settings「技能」page endpoints. All are scoped to the authenticated user:
// custom skills are looked up only in that user's rows; bot usage counts only that user's bots.

// GET /v1/skills → list with source / updated_at / bot_count / bots (superset of the old payload).
func (s *Server) handleListSkillsDetailed(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	list, err := s.db.ListUserSkillsDetailed(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": list})
}

// GET /v1/skills/{name}/package → custom (editable) or built-in (read_only) package with files.
func (s *Server) handleGetSkillPackageView(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	v, err := s.db.GetSkillPackageView(uid, r.PathValue("name"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// POST /v1/skills {name, description?, files?} → create a new custom skill (409 if name taken).
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body struct {
		Name        string               `json:"name"`
		Description string               `json:"description"`
		Files       []db.SkillFileRecord `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxSkillUploadBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	v, err := s.db.CreateUserSkill(uid, body.Name, body.Description, body.Files)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "conflicts") {
			code = http.StatusConflict
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// PUT /v1/skills/{name}/package {files:[{path,content}]} → whole-package write-back (custom only).
func (s *Server) handleSaveSkillPackage(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body struct {
		Files []db.SkillFileRecord `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxSkillUploadBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	v, err := s.db.SaveUserSkillPackage(uid, r.PathValue("name"), body.Files)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "custom skill not found"})
		case errors.Is(err, db.ErrSkillReadOnly):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "内置技能只读，可复制为自建后编辑"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// GET /v1/skills/{name}/export → zip of a custom skill package.
func (s *Server) handleExportSkillZip(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	data, filename, err := s.db.ExportUserSkillZip(uid, r.PathValue("name"))
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "custom skill not found"})
		case errors.Is(err, db.ErrSkillReadOnly):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "built-in skill cannot be exported"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
