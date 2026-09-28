package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserHomeHost is the per-user root under DataRoot: {root}/{sanitized_user}/
func (m *Manager) UserHomeHost(userID string) string {
	return filepath.Join(m.Cfg.DataRoot, SanitizeUserID(userID))
}

// MountRootHost returns the host directory that should be bind-mounted at /workspace.
// Team: {user}/  (contains shared/ + bots/)
// Private: {user}/private/{agent_id}/  (agent home is the whole workspace)
func (m *Manager) MountRootHost(userID, agentID string, mode ComputerMode) string {
	home := m.UserHomeHost(userID)
	mode = NormalizeMode(string(mode))
	if mode == ModePrivate {
		aid := SanitizeUserID(agentID)
		if aid == "" || aid == "unknown" {
			aid = "default"
		}
		return filepath.Join(home, "private", aid)
	}
	return home
}

// SharedHost is …/{user}/shared
func (m *Manager) SharedHost(userID string) string {
	return filepath.Join(m.UserHomeHost(userID), "shared")
}

// BotHost is …/{user}/bots/{agent_id}
func (m *Manager) BotHost(userID, agentID string) string {
	aid := SanitizeUserID(agentID)
	if aid == "" || aid == "unknown" {
		aid = "default"
	}
	return filepath.Join(m.UserHomeHost(userID), "bots", aid)
}

// CheckpointsHost is …/{user}/checkpoints
func (m *Manager) CheckpointsHost(userID string) string {
	return filepath.Join(m.UserHomeHost(userID), "checkpoints")
}

// LatestCheckpointMarker is …/{user}/checkpoints/LATEST (text file with revision dir name)
func (m *Manager) LatestCheckpointMarker(userID string) string {
	return filepath.Join(m.CheckpointsHost(userID), "LATEST")
}

// LegacyWorkspaceHost is the Phase-1 path …/{user}/workspace
func (m *Manager) LegacyWorkspaceHost(userID string) string {
	return filepath.Join(m.UserHomeHost(userID), "workspace")
}

// EnsureLayout creates team/private directories and migrates Phase-1 workspace/ → shared/.
// Migration choice (documented in 沙箱电脑.md):
//   If legacy {user}/workspace exists with content and {user}/shared does not yet exist
//   (or is empty while workspace has files), move workspace contents into shared/, then
//   leave an empty workspace/ directory (or remove it) so we do not wipe user data.
// Mount root for team mode becomes {user}/ so container sees /workspace/shared and /workspace/bots/{id}.
func (m *Manager) EnsureLayout(userID, agentID string, mode ComputerMode) (mountRoot string, err error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user_id required")
	}
	mode = NormalizeMode(string(mode))
	home := m.UserHomeHost(userID)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", err
	}

	if mode == ModeTeam {
		if err := migrateLegacyWorkspace(home); err != nil {
			return "", err
		}
		shared := m.SharedHost(userID)
		if err := os.MkdirAll(shared, 0o755); err != nil {
			return "", err
		}
		bots := filepath.Join(home, "bots")
		if err := os.MkdirAll(bots, 0o755); err != nil {
			return "", err
		}
		if aid := strings.TrimSpace(agentID); aid != "" {
			if err := os.MkdirAll(m.BotHost(userID, aid), 0o755); err != nil {
				return "", err
			}
		}
		_ = os.MkdirAll(m.CheckpointsHost(userID), 0o755)
		return home, nil
	}

	// private
	root := m.MountRootHost(userID, agentID, ModePrivate)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	_ = os.MkdirAll(m.CheckpointsHost(userID), 0o755)
	return root, nil
}

func migrateLegacyWorkspace(home string) error {
	legacy := filepath.Join(home, "workspace")
	shared := filepath.Join(home, "shared")
	st, err := os.Stat(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !st.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	// If shared already has content, leave legacy as-is (do not merge/overwrite).
	sharedExists := false
	sharedHas := false
	if sst, e := os.Stat(shared); e == nil && sst.IsDir() {
		sharedExists = true
		se, _ := os.ReadDir(shared)
		sharedHas = len(se) > 0
	}
	if sharedHas {
		return nil
	}
	if !sharedExists {
		// Fast path: rename workspace → shared
		if err := os.Rename(legacy, shared); err != nil {
			return fmt.Errorf("migrate workspace→shared rename: %w", err)
		}
		_ = os.MkdirAll(legacy, 0o755) // keep empty dir for any old tooling expecting it
		return nil
	}
	// shared exists but empty: move children
	for _, e := range entries {
		src := filepath.Join(legacy, e.Name())
		dst := filepath.Join(shared, e.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("migrate %s: %w", e.Name(), err)
		}
	}
	return nil
}

// RemapWritePath for team mode: bare relative paths (not under shared/ or bots/) go to bots/{agent}/.
// Absolute /workspace/... paths are left alone after clean. Private mode: no remap.
func RemapWritePath(path, agentID string, mode ComputerMode) (string, error) {
	mode = NormalizeMode(string(mode))
	wp, err := ResolveWorkspacePath(path)
	if err != nil {
		return "", err
	}
	if mode != ModeTeam {
		return wp, nil
	}
	rel := strings.TrimPrefix(wp, "/workspace")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		// writing "to workspace root" → bots/{agent}/
		aid := SanitizeUserID(agentID)
		if aid == "" || aid == "unknown" {
			aid = "default"
		}
		return "/workspace/bots/" + aid, nil
	}
	if strings.HasPrefix(rel, "shared/") || rel == "shared" ||
		strings.HasPrefix(rel, "bots/") || rel == "bots" ||
		strings.HasPrefix(rel, "checkpoints/") || rel == "checkpoints" ||
		strings.HasPrefix(rel, "private/") {
		return wp, nil
	}
	aid := SanitizeUserID(agentID)
	if aid == "" || aid == "unknown" {
		aid = "default"
	}
	return filepath.Clean("/workspace/bots/" + aid + "/" + rel), nil
}
