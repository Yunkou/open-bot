package db

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestRemoveChannelMemberRules(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	uid := uuid.NewString()
	if _, err := d.SQL.Exec(`INSERT INTO users (id, username, password_hash, org_id, role, created_at)
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "chmem-"+uid[:8], org.ID, RoleMember, Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })

	lead, err := d.CreateAgent(uid, "lead-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	coder, err := d.CreateAgent(uid, "coder-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := d.CreateAgent(uid, "writer-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}

	ch, err := d.CreateChannel(uid, "群测")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{lead.ID, coder.ID, writer.ID} {
		if err := d.AddChannelMember(uid, ch.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.EnsureChannelConversation(uid, ch.ID, lead.ID, ch.Name); err != nil {
		t.Fatal(err)
	}

	// Happy path: remove non-lead.
	if err := d.RemoveChannelMember(uid, ch.ID, writer.ID); err != nil {
		t.Fatalf("remove writer: %v", err)
	}
	members, _ := d.ListChannelMembers(ch.ID)
	if len(members) != 2 {
		t.Fatalf("want 2 members after remove, got %v", members)
	}
	for _, id := range members {
		if id == writer.ID {
			t.Fatal("writer still present")
		}
	}

	// Cannot remove lead while others remain.
	err = d.RemoveChannelMember(uid, ch.ID, lead.ID)
	if err == nil || err.Error() != "不能移除主助理" {
		t.Fatalf("lead remove want 不能移除主助理, got %v", err)
	}

	// Missing member → ErrNotFound.
	if err := d.RemoveChannelMember(uid, ch.ID, writer.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing member want ErrNotFound, got %v", err)
	}

	// Missing channel → ErrNotFound.
	if err := d.RemoveChannelMember(uid, uuid.NewString(), coder.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing channel want ErrNotFound, got %v", err)
	}

	// Remove coder → only lead left.
	if err := d.RemoveChannelMember(uid, ch.ID, coder.ID); err != nil {
		t.Fatal(err)
	}
	// Last member (lead) cannot be removed — at-least-one fires first.
	err = d.RemoveChannelMember(uid, ch.ID, lead.ID)
	if err == nil || err.Error() != "群里至少保留一位成员" {
		t.Fatalf("last member want 群里至少保留一位成员, got %v", err)
	}
}
