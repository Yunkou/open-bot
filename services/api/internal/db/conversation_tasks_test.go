package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConversationTaskClaimLeaseAndSerial(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	userID := uuid.NewString()
	convID := uuid.NewString()
	username := "task-" + userID[:8]
	if _, err := d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, 'x')`,
		userID, username,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title) VALUES ($1, $2, 'bot', 't')`,
		convID, userID,
	); err != nil {
		t.Fatal(err)
	}

	first, err := d.EnqueueConversationTask(userID, convID, "bot", "补上战斗动画", "m1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := d.EnqueueConversationTask(userID, convID, "bot", "补上战斗动画", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatalf("duplicate goal got %s want %s", again.ID, first.ID)
	}
	if _, err := d.EnqueueConversationTask(userID, convID, "bot", "再加一个背包", "m2"); err != nil {
		t.Fatal(err)
	}

	claimed := claimOurTask(t, d, convID)
	if claimed == nil || claimed.Goal != "补上战斗动画" || claimed.Status != TaskRunning {
		t.Fatalf("claim = %+v", claimed)
	}
	blocked := claimOurTask(t, d, convID)
	if blocked != nil {
		t.Fatalf("second claim ran the same conversation: %+v", blocked)
	}

	ok, err := d.FinishConversationTask(claimed.ID, TaskDone, "")
	if err != nil || !ok {
		t.Fatalf("finish %v %v", ok, err)
	}
	next := claimOurTask(t, d, convID)
	if next == nil || next.Goal != "再加一个背包" {
		t.Fatalf("next = %+v", next)
	}

	if _, err := d.SQL.Exec(
		`UPDATE conversation_tasks SET lease_until = $2 WHERE id = $1`,
		next.ID, time.Now().Add(-time.Minute),
	); err != nil {
		t.Fatal(err)
	}
	if err := d.RequeueExpiredConversationTasks(); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetConversationTask(next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != TaskQueued {
		t.Fatalf("after lease expiry status=%v", got)
	}

	uid, n, err := d.CancelOpenConversationTasks(convID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || uid != userID {
		t.Fatalf("cancel n=%d uid=%s", n, uid)
	}
	open, err := d.OpenConversationTask(convID)
	if err != nil {
		t.Fatal(err)
	}
	if open != nil {
		t.Fatalf("open after cancel: %+v", open)
	}
}

// claimOurTask claims until this conversation's task comes up, putting other
// rows back to queued so a shared dev database is not finished by the test.
func claimOurTask(t *testing.T, d *DB, convID string) *ConversationTask {
	t.Helper()
	var ours *ConversationTask
	var restore []string
	for {
		task, err := d.ClaimNextConversationTask()
		if err != nil {
			t.Fatal(err)
		}
		if task == nil {
			break
		}
		if task.ConversationID == convID {
			ours = task
			break
		}
		restore = append(restore, task.ID)
	}
	for _, id := range restore {
		if _, err := d.SQL.Exec(
			`UPDATE conversation_tasks
			 SET status = $2, lease_until = NULL, started_at = NULL, updated_at = NOW()
			 WHERE id = $1`,
			id, TaskQueued,
		); err != nil {
			t.Fatal(err)
		}
	}
	return ours
}
