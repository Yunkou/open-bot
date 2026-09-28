package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/auth"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	members, err := s.db.ListOrgMembers(admin.OrgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	users := make([]map[string]any, 0, len(members))
	for _, m := range members {
		users = append(users, map[string]any{
			"id":         m.ID,
			"username":   m.Username,
			"email":      m.Email,
			"role":       m.Role,
			"org_id":     m.OrgID,
			"created_at": m.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type adminCreateUserBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	var body adminCreateUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	username := strings.TrimSpace(body.Username)
	password := body.Password
	if username == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username 与 password 必填"})
		return
	}
	if strings.EqualFold(username, db.A2ASystemUsername) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "保留用户名不可用"})
		return
	}
	role := db.NormalizeRole(body.Role)
	if role == db.RolePlatformAdmin && !db.RoleAtLeast(admin.Role, db.RolePlatformAdmin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "仅平台管理员可授予 platform_admin"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hash failed"})
		return
	}
	u, err := s.db.CreateUserFull(username, hash, body.Email, "", db.RoleMember)
	if err != nil {
		if errors.Is(err, db.ErrUserExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "用户名已存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.db.SetUserOrgRole(u.ID, admin.OrgID, role); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	u, err = s.db.GetUserByID(u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "user.create", "user", u.ID, map[string]any{
		"username": u.Username,
		"role":     u.Role,
		"email":    u.Email,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"user": u.PublicMap()})
}

type adminPatchUserBody struct {
	Email    *string `json:"email"`
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

func (s *Server) handleAdminPatchUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("id")
	var body adminPatchUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	target, err := s.db.GetUserByID(userID)
	if err != nil || target.OrgID != admin.OrgID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
		return
	}
	if strings.EqualFold(target.Username, db.A2ASystemUsername) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "不可修改系统用户"})
		return
	}

	meta := map[string]any{"username": target.Username}

	if body.Role != nil {
		role := db.NormalizeRole(*body.Role)
		if role == db.RolePlatformAdmin && !db.RoleAtLeast(admin.Role, db.RolePlatformAdmin) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "仅平台管理员可授予 platform_admin"})
			return
		}
		// Refuse demoting yourself out of admin.
		if target.ID == admin.ID && !db.IsAdminRole(role) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能将自己降为非管理员"})
			return
		}
		// Refuse demoting/removing the last platform_admin.
		if target.Role == db.RolePlatformAdmin && role != db.RolePlatformAdmin {
			n, err := s.db.CountPlatformAdmins()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if n <= 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能降级唯一的平台管理员"})
				return
			}
		}
		prev := target.Role
		if err := s.db.SetUserOrgRole(target.ID, admin.OrgID, role); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		meta["from_role"] = prev
		meta["to_role"] = role
	}

	if body.Email != nil {
		if err := s.db.UpdateUserEmail(target.ID, *body.Email); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		meta["email"] = strings.TrimSpace(*body.Email)
	}

	if body.Password != nil {
		pw := *body.Password
		if strings.TrimSpace(pw) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password 不能为空"})
			return
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hash failed"})
			return
		}
		if err := s.db.SetUserPasswordHash(target.ID, hash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		meta["password_reset"] = true
	}

	refreshed, err := s.db.GetUserByID(target.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "user.update", "user", refreshed.ID, meta)
	writeJSON(w, http.StatusOK, map[string]any{"user": refreshed.PublicMap()})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
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
	if target.ID == admin.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能删除自己"})
		return
	}
	if strings.EqualFold(target.Username, db.A2ASystemUsername) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "不可删除系统用户"})
		return
	}
	if target.Role == db.RolePlatformAdmin {
		n, err := s.db.CountPlatformAdmins()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if n <= 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不能删除唯一的平台管理员"})
			return
		}
	}
	if err := s.db.DeleteUser(target.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "user.delete", "user", target.ID, map[string]any{
		"username": target.Username,
		"role":     target.Role,
	})
	w.WriteHeader(http.StatusNoContent)
}
