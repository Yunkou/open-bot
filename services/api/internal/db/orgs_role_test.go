package db

import "testing"

func TestIsAdminRole(t *testing.T) {
	if IsAdminRole(RoleMember) {
		t.Fatal("member must not be admin")
	}
	if !IsAdminRole(RoleOrgAdmin) {
		t.Fatal("org_admin must be admin")
	}
	if !IsAdminRole(RolePlatformAdmin) {
		t.Fatal("platform_admin must be admin")
	}
	if !IsAdminRole("ORG_ADMIN") {
		t.Fatal("NormalizeRole should accept ORG_ADMIN")
	}
}

func TestNormalizeRoleDefaultsMember(t *testing.T) {
	if got := NormalizeRole(""); got != RoleMember {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeRole("nope"); got != RoleMember {
		t.Fatalf("got %q", got)
	}
}
