package httpserver

import (
	"testing"
	"time"
)

func TestCronMatchesMinuteInTZ(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-09-30 09:00 CST
	now := time.Date(2026, 9, 30, 9, 0, 0, 0, loc)
	if !cronMatchesMinute("0 9 * * *", now) {
		t.Fatal("should match 09:00")
	}
	if cronMatchesMinute("0 10 * * *", now) {
		t.Fatal("should not match 10:00")
	}
	if cronMatchesMinute("", now) {
		t.Fatal("empty cron")
	}
}
