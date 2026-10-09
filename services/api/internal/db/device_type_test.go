package db

import "testing"

func TestNormalizeDeviceType(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", DeviceTypeDesktop},
		{"desktop", DeviceTypeDesktop},
		{"Desktop", DeviceTypeDesktop},
		{"mobile", DeviceTypeMobile},
		{"MOBILE", DeviceTypeMobile},
		{"phone", DeviceTypeDesktop}, // unknown → desktop (legacy-safe)
		{"  mobile  ", DeviceTypeMobile},
	}
	for _, tc := range cases {
		if got := NormalizeDeviceType(tc.in); got != tc.want {
			t.Fatalf("NormalizeDeviceType(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestInferDeviceType(t *testing.T) {
	cases := []struct {
		platform, os, app, want string
	}{
		{"macos", "darwin", "open-bot-desktop", DeviceTypeDesktop},
		{"windows", "windows", "electron", DeviceTypeDesktop},
		{"linux", "linux", "open-bot", DeviceTypeDesktop},
		{"web", "", "browser", DeviceTypeDesktop},
		{"android", "android", "open-bot", DeviceTypeMobile},
		{"ios", "ios", "open-bot", DeviceTypeMobile},
		{"", "iPhone OS", "", DeviceTypeMobile},
		{"", "", "capacitor", DeviceTypeMobile},
		{"", "iPadOS", "app", DeviceTypeMobile},
		{"unknown", "", "", DeviceTypeDesktop},
	}
	for _, tc := range cases {
		got := InferDeviceType(tc.platform, tc.os, tc.app)
		if got != tc.want {
			t.Fatalf("Infer(%q,%q,%q)=%q want %q", tc.platform, tc.os, tc.app, got, tc.want)
		}
	}
}

func TestResolveDeviceType(t *testing.T) {
	// Phone signals win over a buggy explicit "desktop" (rule A: login-only).
	if got := ResolveDeviceType("desktop", "android", "android", "capacitor"); got != DeviceTypeMobile {
		t.Fatalf("android+capacitor must be mobile, got %q", got)
	}
	if got := ResolveDeviceType("desktop", "ios", "", ""); got != DeviceTypeMobile {
		t.Fatalf("ios must be mobile even if explicit desktop: %q", got)
	}
	if got := ResolveDeviceType("mobile", "macos", "", ""); got != DeviceTypeMobile {
		t.Fatalf("explicit mobile on macos: %q", got)
	}
	if got := ResolveDeviceType("desktop", "macos", "darwin", "tauri"); got != DeviceTypeDesktop {
		t.Fatalf("explicit desktop on macos: %q", got)
	}
	if got := ResolveDeviceType("", "android", "", "app"); got != DeviceTypeMobile {
		t.Fatalf("infer android: %q", got)
	}
}

func TestIsHostEligible(t *testing.T) {
	if !IsHostEligible(Machine{DeviceType: DeviceTypeDesktop}) {
		t.Fatal("desktop should be host eligible")
	}
	if IsHostEligible(Machine{DeviceType: DeviceTypeMobile}) {
		t.Fatal("mobile must not be host eligible")
	}
	if !IsHostEligible(Machine{DeviceType: ""}) {
		t.Fatal("empty device_type (legacy) defaults to desktop host")
	}
}

func TestApplyDeviceFields(t *testing.T) {
	m := Machine{DeviceType: "MOBILE"}
	m.ApplyDeviceFields()
	if m.DeviceType != DeviceTypeMobile || m.HostEligible {
		t.Fatalf("got type=%q eligible=%v", m.DeviceType, m.HostEligible)
	}
	m2 := Machine{DeviceType: ""}
	m2.ApplyDeviceFields()
	if m2.DeviceType != DeviceTypeDesktop || !m2.HostEligible {
		t.Fatalf("legacy empty: type=%q eligible=%v", m2.DeviceType, m2.HostEligible)
	}
}
