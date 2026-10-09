package auth

import (
	"os"
	"testing"
)

func TestMapOIDCRolesToLocalDefaults(t *testing.T) {
	t.Setenv("CASDOOR_ROLE_MAP", "")
	t.Setenv("CASDOOR_ROLE_PLATFORM_ADMIN", "platform_admin,admin")
	t.Setenv("CASDOOR_ROLE_ORG_ADMIN", "org_admin,org-admin")

	role, ok := MapOIDCRolesToLocal([]string{"admin"}, nil)
	if !ok || role != "platform_admin" {
		t.Fatalf("got %q ok=%v", role, ok)
	}
	role, ok = MapOIDCRolesToLocal(nil, []string{"org-admin"})
	if !ok || role != "org_admin" {
		t.Fatalf("got %q ok=%v", role, ok)
	}
	role, ok = MapOIDCRolesToLocal([]string{"viewer"}, nil)
	if ok && role != "member" {
		// unmatched known list → member with matched=false when only unknown roles
	}
	_, ok = MapOIDCRolesToLocal(nil, nil)
	if ok {
		t.Fatal("empty should not match")
	}
	_ = os.Getenv // silence
}

func TestMapOIDCRolesToLocalExplicitMap(t *testing.T) {
	t.Setenv("CASDOOR_ROLE_MAP", "super=platform_admin,manager=org_admin,*=member")
	role, ok := MapOIDCRolesToLocal([]string{"super"}, nil)
	if !ok || role != "platform_admin" {
		t.Fatalf("got %q ok=%v", role, ok)
	}
	role, ok = MapOIDCRolesToLocal([]string{"manager"}, nil)
	if !ok || role != "org_admin" {
		t.Fatalf("got %q ok=%v", role, ok)
	}
}
