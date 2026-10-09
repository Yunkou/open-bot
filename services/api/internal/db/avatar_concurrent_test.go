package db

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// Regression: parallel creates for one user must not collide on (shape,color).
func TestCreateAgentConcurrentUniqueAvatars(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.NewString()
	if _, err := d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
		userID, "avatar-conc-"+userID[:8], org.ID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id = $1`, userID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := d.CreateAgent(userID, fmt.Sprintf("conc-%d", i), "", ""); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	rows, err := d.SQL.Query(`SELECT avatar_shape, avatar_color FROM agents WHERE user_id = $1 AND deleted_at IS NULL`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	total := 0
	for rows.Next() {
		var s, c string
		if err := rows.Scan(&s, &c); err != nil {
			t.Fatal(err)
		}
		seen[s+"|"+c]++
		total++
	}
	if total != n {
		t.Fatalf("created %d agents, want %d", total, n)
	}
	if len(seen) != n {
		t.Fatalf("unique avatar pairs=%d, want %d (collisions: %v)", len(seen), n, seen)
	}
}
