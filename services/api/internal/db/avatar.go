package db

import (
	"hash/fnv"
	"strings"
)

var AllowedAvatarShapes = []string{
	"circle", "rounded", "squircle", "hex", "diamond", "soft-square",
}

var AllowedAvatarColors = []string{
	"#e85d4c", "#2a9d8f", "#f4a261", "#e76f51", "#457b9d", "#9b5de5",
	"#00bbf9", "#f15bb5", "#00f5d4", "#fee440", "#06d6a0", "#118ab2",
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
	// walk palette for a free pair
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
