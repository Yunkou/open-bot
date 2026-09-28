// Routines worker: polls due routines and triggers API internal run path.
// Prefer ROUTINES_INPROCESS=0 on the API when running this worker to avoid double-fire.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func main() {
	apiURL := strings.TrimRight(env("OPENBOT_API_URL", "http://127.0.0.1:18080"), "/")
	token := env("INTERNAL_TOKEN", "open-bot-dev-internal")
	interval := 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("WORKER_POLL_SECONDS")); v != "" {
		if n, err := time.ParseDuration(v + "s"); err == nil && n >= time.Second {
			interval = n
		}
	}

	database, err := db.Open(db.DatabaseURL())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	log.Printf("routines worker started poll=%s api=%s", interval, apiURL)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	tick(database, apiURL, token)
	for range ticker.C {
		tick(database, apiURL, token)
	}
}

func tick(database *db.DB, apiURL, token string) {
	list, err := database.ClaimDueRoutines()
	if err != nil {
		log.Printf("list routines: %v", err)
		return
	}
	now := time.Now()
	for _, rt := range list {
		if !cronMatchesMinute(rt.ScheduleCron, now) {
			continue
		}
		if rt.LastRunAt != nil {
			lr := rt.LastRunAt.In(now.Location())
			if lr.Year() == now.Year() && lr.Month() == now.Month() && lr.Day() == now.Day() &&
				lr.Hour() == now.Hour() && lr.Minute() == now.Minute() {
				continue
			}
		}
		log.Printf("claim routine %s (%s)", rt.ID, rt.Name)
		if err := triggerRun(apiURL, token, rt.ID); err != nil {
			log.Printf("routine %s failed: %v", rt.ID, err)
			next := nextCron(rt.ScheduleCron, now)
			_ = database.MarkRoutineSchedule(rt.ID, rt.LastRunAt, next, err.Error())
			continue
		}
		next := nextCron(rt.ScheduleCron, now)
		ts := now.UTC()
		_ = database.MarkRoutineSchedule(rt.ID, &ts, next, "")
		log.Printf("routine %s ok next=%v", rt.ID, next)
	}
}

func triggerRun(apiURL, token, routineID string) error {
	payload, _ := json.Marshal(map[string]string{"routine_id": routineID})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/internal/routines/run", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func cronMatchesMinute(expr string, now time.Time) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return false
	}
	prev := now.Add(-time.Second)
	next := sched.Next(prev)
	return next.Year() == now.Year() &&
		next.Month() == now.Month() &&
		next.Day() == now.Day() &&
		next.Hour() == now.Hour() &&
		next.Minute() == now.Minute()
}

func nextCron(expr string, now time.Time) *time.Time {
	sched, err := cronParser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return nil
	}
	n := sched.Next(now)
	return &n
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
