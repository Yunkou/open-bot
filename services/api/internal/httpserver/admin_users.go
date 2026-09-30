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

// rejectDeleteUser reports why an org admin may not soft-delete target.
// Platform-admin last-one protection is applied by the caller, because batch
// delete must count the whole set.
func rejectDeleteUser(admin, target *db.User) (int, string) {
	if target == nil || target.OrgID != admin.OrgID {
		return http.StatusNotFound, "用户不存在"
	}
	if target.ID == admin.ID {
		return http.StatusBadRequest, "不能删除自己"
	}
	if strings.EqualFold(target.Username, db.A2ASystemUsername) {
		return http.StatusForbidden, "不可删除系统用户"
	}
	return 0, ""
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	target, err := s.db.GetUserByID(r.PathValue("id"))
	if err != nil || target.OrgID != admin.OrgID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
		return
	}
	if code, msg := rejectDeleteUser(admin, target); code != 0 {
		writeJSON(w, code, map[string]string{"error": msg})
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
	agentIDs, err := s.db.SoftDeleteUser(target.ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.abortUserTasks(target.ID)
	s.writeAudit(admin.OrgID, admin.ID, "user.delete", "user", target.ID, map[string]any{
		"username":  target.Username,
		"role":      target.Role,
		"agent_ids": agentIDs,
		"soft":      true,
	})
	w.WriteHeader(http.StatusNoContent)
}

type adminBatchDeleteUsersBody struct {
	IDs []string `json:"ids"`
}

type adminDeleteFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

func (s *Server) handleAdminBatchDeleteUsers(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	var body adminBatchDeleteUsersBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(body.IDs))
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ids 必填"})
		return
	}
	if len(ids) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "一次最多删除 100 个用户"})
		return
	}

	failed := make([]adminDeleteFailure, 0)
	eligible := make([]*db.User, 0, len(ids))
	for _, id := range ids {
		target, err := s.db.GetUserByID(id)
		if err != nil || target.OrgID != admin.OrgID {
			failed = append(failed, adminDeleteFailure{ID: id, Error: "用户不存在"})
			continue
		}
		if _, msg := rejectDeleteUser(admin, target); msg != "" {
			failed = append(failed, adminDeleteFailure{ID: id, Error: msg})
			continue
		}
		eligible = append(eligible, target)
	}

	deletingAdmins := 0
	for _, t := range eligible {
		if t.Role == db.RolePlatformAdmin {
			deletingAdmins++
		}
	}
	if deletingAdmins > 0 {
		n, err := s.db.CountPlatformAdmins()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if n-deletingAdmins < 1 {
			// Keep at least one live platform_admin. Extra ones in this batch are skipped.
			allowed := n - 1
			if allowed < 0 {
				allowed = 0
			}
			queued := 0
			kept := make([]*db.User, 0, len(eligible))
			for _, t := range eligible {
				if t.Role == db.RolePlatformAdmin {
					if queued >= allowed {
						failed = append(failed, adminDeleteFailure{ID: t.ID, Error: "不能删除唯一的平台管理员"})
						continue
					}
					queued++
				}
				kept = append(kept, t)
			}
			eligible = kept
		}
	}

	deleted := make([]map[string]any, 0, len(eligible))
	for _, target := range eligible {
		agentIDs, err := s.db.SoftDeleteUser(target.ID)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, db.ErrNotFound) {
				msg = "用户不存在"
			}
			failed = append(failed, adminDeleteFailure{ID: target.ID, Error: msg})
			continue
		}
		s.abortUserTasks(target.ID)
		s.writeAudit(admin.OrgID, admin.ID, "user.delete", "user", target.ID, map[string]any{
			"username":  target.Username,
			"role":      target.Role,
			"agent_ids": agentIDs,
			"soft":      true,
			"batch":     true,
		})
		deleted = append(deleted, map[string]any{
			"id":        target.ID,
			"username":  target.Username,
			"agent_ids": agentIDs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": deleted,
		"failed":  failed,
	})
}

func (s *Server) handleAdminListUserMachines(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.db.ListMachines(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []db.Machine{}
	}
	s.markConnected(userID, list)
	writeJSON(w, http.StatusOK, map[string]any{
		"machines": list,
		"user_id":  userID,
		"username": target.Username,
	})
}

func (s *Server) handleAdminDeleteUserMachine(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	userID := r.PathValue("id")
	machineID := r.PathValue("machineId")
	target, err := s.db.GetUserByID(userID)
	if err != nil || target.OrgID != admin.OrgID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
		return
	}
	if err := s.db.DeleteMachine(userID, machineID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "设备不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeAudit(admin.OrgID, admin.ID, "user.machine_delete", "user", userID, map[string]any{
		"machine_id": machineID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
