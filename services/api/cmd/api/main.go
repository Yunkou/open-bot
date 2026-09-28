package main

import (
	"log"
	"os"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/auth"
	"github.com/tangxin/open-bot/services/api/internal/db"
	"github.com/tangxin/open-bot/services/api/internal/httpserver"
)

func main() {
	addr := env("API_ADDR", ":18080")
	runtimeURL := env("AGENT_RUNTIME_URL", "http://127.0.0.1:8001")
	databaseURL := db.DatabaseURL()

	database, err := db.Open(databaseURL)
	if err != nil {
		log.Fatalf("database: %v (start postgres: make compose-postgres)", err)
	}
	defer database.Close()

	if err := maybeBootstrapAdmin(database); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	log.Printf("open-bot api listening on %s (runtime=%s, jwt_secret_set=%v)", addr, runtimeURL, os.Getenv("JWT_SECRET") != "" || auth.Secret() != "")
	if err := httpserver.Listen(addr, runtimeURL, database); err != nil {
		log.Fatal(err)
	}
}

func maybeBootstrapAdmin(database *db.DB) error {
	username := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	if username == "" || password == "" {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	u, created, err := database.BootstrapPlatformAdmin(username, hash, email)
	if err != nil {
		return err
	}
	if u == nil {
		return nil
	}
	if created {
		log.Printf("bootstrapped platform_admin %q in default org", u.Username)
	} else {
		log.Printf("ensured platform_admin on existing user %q", u.Username)
	}
	return nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
