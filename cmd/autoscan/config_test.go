package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestConfigDecodesSiloTarget(t *testing.T) {
	const input = "targets:\n  silo:\n    - url: http://silo.test:8080\n      token: test-token\n"
	var cfg config
	decoder := yaml.NewDecoder(strings.NewReader(input))
	decoder.SetStrict(true)
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatalf("strict config decode error = %v, want nil", err)
	}
	if len(cfg.Targets.Silo) != 1 {
		t.Fatalf("Silo target count = %d, want 1", len(cfg.Targets.Silo))
	}
	if got := cfg.Targets.Silo[0].URL; got != "http://silo.test:8080" {
		t.Errorf("Silo URL = %q, want http://silo.test:8080", got)
	}
	if got := cfg.Targets.Silo[0].Token; got != "test-token" {
		t.Errorf("Silo token = %q, want test-token", got)
	}
}
