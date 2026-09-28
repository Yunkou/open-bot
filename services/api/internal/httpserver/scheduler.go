package httpserver

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// StartRoutineScheduler ticks every minute inside the API process and runs due routines.
func (s *Server) StartRoutineScheduler(ctx context.Context) {
	go func() {
		// Align to next minute boundary.
		now := time.Now()
		wait := time.Until(now.Truncate(time.Minute).Add(time.Minute))
		if wait > 0 && wait < time.Minute+time.Second {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		s.tickRoutines(time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				s.tickRoutines(t)
			}
		}
	}()
	log.Printf("routines scheduler started (in-process, 1m tick)")
}

func (s *Server) tickRoutines(now time.Time) {
	list, err := s.db.ListEnabledRoutines()
	if err != nil {
		log.Printf("routines tick list error: %v", err)
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for _, rt := range list {
		if !cronMatchesMinute(rt.ScheduleCron, now) {
			continue
		}
		// Skip if already ran in this minute (restart / double tick).
		if rt.LastRunAt != nil {
			lr := rt.LastRunAt.In(now.Location())
			if lr.Year() == now.Year() && lr.Month() == now.Month() && lr.Day() == now.Day() &&
				lr.Hour() == now.Hour() && lr.Minute() == now.Minute() {
				continue
			}
		}
		rt := rt
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			defer cancel()
			if _, err := s.executeRoutine(ctx, rt); err != nil {
				log.Printf("routine %s (%s) failed: %v", rt.ID, rt.Name, err)
			} else {
				log.Printf("routine %s (%s) ok", rt.ID, rt.Name)
			}
		}()
	}
	wg.Wait()
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
	// Next after (now - 1s) should land in the current minute.
	prev := now.Add(-time.Second)
	next := sched.Next(prev)
	return next.Year() == now.Year() &&
		next.Month() == now.Month() &&
		next.Day() == now.Day() &&
		next.Hour() == now.Hour() &&
		next.Minute() == now.Minute()
}
