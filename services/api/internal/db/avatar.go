package db

import (
	"hash/fnv"
	"strings"
)

// AllowedAvatarShapes is the v2 silhouette whitelist (organic blobs; no letter/ring).
var AllowedAvatarShapes = []string{
	"cloud", "bean", "drop", "soft-hex", "petal", "puff",
}

var AllowedAvatarColors = []string{
	"#e85d4c", "#2a9d8f", "#f4a261", "#e76f51", "#457b9d", "#9b5de5",
	"#00bbf9", "#f15bb5", "#00f5d4", "#fee440", "#06d6a0", "#118ab2",
}

// legacyAvatarShapeMap maps v1 geometric ids → v2 silhouettes (1:1).
var legacyAvatarShapeMap = map[string]string{
	"circle":      "cloud",
	"rounded":     "puff",
	"squircle":    "bean",
	"hex":         "soft-hex",
	"diamond":     "drop",
	"soft-square": "petal",
}

var allowedShapeSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedAvatarShapes))
	for _, s := range AllowedAvatarShapes {
		m[s] = struct{}{}
	}
	return m
}()

var allowedColorSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedAvatarColors))
	for _, c := range AllowedAvatarColors {
		m[strings.ToLower(c)] = struct{}{}
	}
	return m
}()

// ErrAvatarNotAllowed marks a whitelist rejection (maps to HTTP 400).
type ErrAvatarNotAllowed string

func (e ErrAvatarNotAllowed) Error() string { return string(e) }

func IsAllowedAvatarShape(shape string) bool {
	_, ok := allowedShapeSet[strings.TrimSpace(shape)]
	return ok
}

func IsAllowedAvatarColor(color string) bool {
	_, ok := allowedColorSet[strings.ToLower(strings.TrimSpace(color))]
	return ok
}

func NormalizeAvatarColor(color string) string {
	c := strings.ToLower(strings.TrimSpace(color))
	for _, allowed := range AllowedAvatarColors {
		if strings.ToLower(allowed) == c {
			return allowed
		}
	}
	return ""
}

// MapLegacyAvatarShape returns the v2 silhouette for a v1 geometric id, or "".
func MapLegacyAvatarShape(shape string) string {
	return legacyAvatarShapeMap[strings.TrimSpace(shape)]
}

// CanonicalAvatarShape returns an allowed v2 shape: passthrough, legacy map, or "".
func CanonicalAvatarShape(shape string) string {
	shape = strings.TrimSpace(shape)
	if IsAllowedAvatarShape(shape) {
		return shape
	}
	if mapped := MapLegacyAvatarShape(shape); mapped != "" {
		return mapped
	}
	return ""
}

// AvatarAssignLockKey is a stable int64 for pg_advisory_xact_lock per user.
func AvatarAssignLockKey(userID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("avatar-assign:" + userID))
	return int64(h.Sum64())
}

// AssignAvatarFromID picks shape+color from agent id hash.
func AssignAvatarFromID(agentID string) (shape, color string) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentID))
	v := h.Sum32()
	shape = AllowedAvatarShapes[int(v)%len(AllowedAvatarShapes)]
	color = AllowedAvatarColors[int(v/uint32(len(AllowedAvatarShapes)))%len(AllowedAvatarColors)]
	return shape, color
}

// AssignAvatarAvoiding tries to avoid shape+color pairs already used by the same user.
func AssignAvatarAvoiding(agentID string, usedPairs map[string]struct{}) (shape, color string) {
	shape, color = AssignAvatarFromID(agentID)
	key := shape + "|" + color
	if _, hit := usedPairs[key]; !hit {
		return shape, color
	}
	for i := 0; i < len(AllowedAvatarShapes)*len(AllowedAvatarColors); i++ {
		s := AllowedAvatarShapes[i%len(AllowedAvatarShapes)]
		c := AllowedAvatarColors[(i/len(AllowedAvatarShapes))%len(AllowedAvatarColors)]
		k := s + "|" + c
		if _, hit := usedPairs[k]; !hit {
			return s, c
		}
	}
	return shape, color
}
