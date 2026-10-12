package db

import (
	"testing"

	"github.com/google/uuid"
)

func TestResolveEffectiveLLMPrefersOrgOverEnvFallback(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	slug := "llmres-" + uuid.NewString()[:8]
	org, err := d.CreateOrg(slug, "LLM resolve test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM org_settings WHERE org_id=$1`, org.ID)
		_, _ = d.SQL.Exec(`DELETE FROM orgs WHERE id=$1`, org.ID)
	})

	uid := uuid.NewString()
	if _, err := d.SQL.Exec(`INSERT INTO users (id, username, password_hash, org_id, role, created_at)
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "llmres-"+uid[:8], org.ID, RoleMember, Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM llm_connections WHERE user_id=$1`, uid)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid)
	})

	wantModel := "gpt-admin-priority-" + uid[:8]
	if _, err := d.UpsertOrgLLM(org.ID, "组织默认", "https://example.test/v1", "sk-test", wantModel, true, nil, nil, false); err != nil {
		t.Fatal(err)
	}

	conn, err := d.ResolveEffectiveLLM(uid)
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected org LLM, got nil (would fall through to env OPENAI_MODEL)")
	}
	if conn.Model != wantModel {
		t.Fatalf("model=%q want %q", conn.Model, wantModel)
	}

	// Soft-deleted user must still resolve admin org LLM (not env MiniMax).
	if _, err := d.SQL.Exec(`UPDATE users SET deleted_at=$1 WHERE id=$2`, Now(), uid); err != nil {
		t.Fatal(err)
	}
	conn2, err := d.ResolveEffectiveLLM(uid)
	if err != nil {
		t.Fatal(err)
	}
	if conn2 == nil || conn2.Model != wantModel {
		t.Fatalf("soft-deleted resolve: %#v err=%v", conn2, err)
	}
}

func TestOrgSettingsAsLLMRequiresModel(t *testing.T) {
	s := &OrgSettings{OrgID: "o1", LLMBaseURL: "https://example.test/v1", LLMAPIKey: "sk", LLMModel: ""}
	if OrgSettingsAsLLM(s, "u1") != nil {
		t.Fatal("empty model must not yield override (env fallback trap)")
	}
	s.LLMModel = "gpt-6-luna"
	got := OrgSettingsAsLLM(s, "u1")
	if got == nil || got.Model != "gpt-6-luna" {
		t.Fatalf("got %#v", got)
	}
	// model-only still counts (base/key may come from env via partial override)
	only := OrgSettingsAsLLM(&OrgSettings{OrgID: "o1", LLMModel: "gpt-6-luna"}, "u1")
	if only == nil || only.Model != "gpt-6-luna" {
		t.Fatalf("model-only: %#v", only)
	}
}
