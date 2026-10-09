package db

import "testing"

func TestNormalizeDecisionProvider(t *testing.T) {
	if got := NormalizeDecisionProvider(" Jev "); got != "jev" {
		t.Fatalf("jev: %s", got)
	}
	if got := NormalizeDecisionProvider("LAYA"); got != "laya" {
		t.Fatalf("laya: %s", got)
	}
	if got := NormalizeDecisionProvider(""); got != "off" {
		t.Fatalf("empty: %s", got)
	}
	if got := NormalizeDecisionProvider("mem0"); got != "off" {
		t.Fatalf("unknown: %s", got)
	}
}

func TestApplyDecisionDefaults(t *testing.T) {
	p, base, model := ApplyDecisionDefaults("off", "", "")
	if p != "off" || base != "" || model != "" {
		t.Fatalf("off should stay empty: %s %s %s", p, base, model)
	}
	p, base, model = ApplyDecisionDefaults("jev", "", "")
	if p != "jev" || base != "https://api.typesafe.ai" || model != "jev-latest" {
		t.Fatalf("jev defaults: %s %s %s", p, base, model)
	}
	p, base, model = ApplyDecisionDefaults("jev", "https://example.test/", "jev-1.13.0")
	if base != "https://example.test" || model != "jev-1.13.0" {
		t.Fatalf("jev custom: %s %s", base, model)
	}
	p, base, model = ApplyDecisionDefaults("laya", "", "")
	if p != "laya" || base != "" || model != "convaiinnovations/laya" {
		t.Fatalf("laya local: %s %q %s", p, base, model)
	}
	_, base, _ = ApplyDecisionDefaults("laya", "http://127.0.0.1:8081/", "")
	if base != "http://127.0.0.1:8081" {
		t.Fatalf("laya http base: %s", base)
	}
}
