package httpserver

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHostConfirmStatus(t *testing.T) {
	if hostConfirmStatus(map[string]any{"denied": true, "ok": false}) != "denied" {
		t.Fatal("denied")
	}
	if hostConfirmStatus(map[string]any{"ok": true}) != "allowed" {
		t.Fatal("ok")
	}
	if hostConfirmStatus(map[string]any{"ok": false, "confirmed": true}) != "allowed" {
		t.Fatal("confirmed failure still counts as allowed")
	}
	if hostConfirmStatus(map[string]any{"ok": false}) != "denied" {
		t.Fatal("timeout")
	}
}

func TestHostCallNotConnected(t *testing.T) {
	h := newHostHub()
	_, err := h.Call(context.Background(), "user", "machine", hostExecRequest{Op: "ls", Path: "Downloads"})
	if !errors.Is(err, errHostNotConnected) {
		t.Fatalf("expected not connected, got %v", err)
	}
}

func TestHostCallDeliversResult(t *testing.T) {
	h := newHostHub()
	sess := &hostSession{
		userID:    "user",
		machineID: "machine",
		send:      make(chan []byte, 1),
		pending:   map[string]chan map[string]any{},
	}
	h.sessions[hostKey("user", "machine")] = sess
	go func() {
		raw := <-sess.send
		if len(raw) == 0 {
			t.Errorf("empty request")
		}
		sess.mu.Lock()
		var reqID string
		for id := range sess.pending {
			reqID = id
		}
		sess.mu.Unlock()
		h.deliver(sess, reqID, map[string]any{"ok": true, "entries": []any{"a.txt"}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := h.Call(ctx, "user", "machine", hostExecRequest{Op: "ls", Path: "Downloads"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := got["ok"].(bool); !ok {
		t.Fatalf("result %+v", got)
	}
}
