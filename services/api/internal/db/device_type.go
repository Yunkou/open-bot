package db

import (
	"errors"
	"strings"
)

// Device types for user_machines. Phones are login-only; only desktop may host.
const (
	DeviceTypeDesktop = "desktop"
	DeviceTypeMobile  = "mobile"
)

// ErrMobileNotHost is returned when binding agents.machine_id to a mobile device.
var ErrMobileNotHost = errors.New("mobile devices cannot be preferred hosts")

// NormalizeDeviceType maps client input to desktop|mobile. Empty/unknown → desktop (legacy).
func NormalizeDeviceType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case DeviceTypeMobile:
		return DeviceTypeMobile
	default:
		return DeviceTypeDesktop
	}
}

// InferDeviceType guesses desktop|mobile from platform/os/app when device_type is omitted.
func InferDeviceType(platform, osName, app string) string {
	blob := strings.ToLower(strings.TrimSpace(platform) + " " + strings.TrimSpace(osName) + " " + strings.TrimSpace(app))
	for _, needle := range []string{"android", "ios", "iphone", "ipad", "capacitor"} {
		if strings.Contains(blob, needle) {
			return DeviceTypeMobile
		}
	}
	return DeviceTypeDesktop
}

// ResolveDeviceType maps register input to desktop|mobile.
// If platform/os/app clearly look like a phone (android/ios/capacitor), always
// return mobile — even if a buggy client sends device_type=desktop. Phones are
// login-only and must never become prefer-host targets.
func ResolveDeviceType(explicit, platform, osName, app string) string {
	inferred := InferDeviceType(platform, osName, app)
	if inferred == DeviceTypeMobile {
		return DeviceTypeMobile
	}
	if strings.TrimSpace(explicit) != "" {
		return NormalizeDeviceType(explicit)
	}
	return inferred
}

// IsHostEligible is true only for desktop machines (host exec / green-dot / preferred).
func IsHostEligible(m Machine) bool {
	return NormalizeDeviceType(m.DeviceType) == DeviceTypeDesktop
}

// ApplyDeviceFields normalizes device_type and sets computed host_eligible.
func (m *Machine) ApplyDeviceFields() {
	if m == nil {
		return
	}
	m.DeviceType = NormalizeDeviceType(m.DeviceType)
	m.HostEligible = m.DeviceType == DeviceTypeDesktop
}
