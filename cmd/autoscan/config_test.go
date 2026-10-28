package main

import (
	"strings"
	"testing"
)

func TestConfigDecodesSiloTarget(t *testing.T) {
	const input = "targets:\n  silo:\n    - url: http://silo.test:8080\n      token: test-token\n"
	cfg, err := decodeConfig(strings.NewReader(input))
	if err != nil {
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

func TestConfigDecodesSiloRewrite(t *testing.T) {
	const input = `targets:
  silo:
    - url: http://silo.test:8080
      token: test-token
      rewrite:
        - from: /mnt/unionfs/
          to: /mnt/media/
`
	cfg, err := decodeConfig(strings.NewReader(input))
	if err != nil {
		t.Fatalf("strict config decode error = %v, want nil", err)
	}
	if len(cfg.Targets.Silo) != 1 {
		t.Fatalf("Silo target count = %d, want 1", len(cfg.Targets.Silo))
	}
	got := cfg.Targets.Silo[0]
	if len(got.Rewrite) != 1 {
		t.Fatalf("rewrite count = %d, want 1", len(got.Rewrite))
	}
	if rule := got.Rewrite[0]; rule.From != "/mnt/unionfs/" || rule.To != "/mnt/media/" {
		t.Errorf("rewrite = %#v, want /mnt/unionfs/ to /mnt/media/", rule)
	}
	if got.Verbosity != "" {
		t.Errorf("verbosity = %q, want empty", got.Verbosity)
	}
}

func TestConfigDecodesSiloVerbosity(t *testing.T) {
	const input = `targets:
  silo:
    - url: http://silo.test:8080
      token: test-token
      verbosity: debug
`
	cfg, err := decodeConfig(strings.NewReader(input))
	if err != nil {
		t.Fatalf("strict config decode error = %v, want nil", err)
	}
	if len(cfg.Targets.Silo) != 1 {
		t.Fatalf("Silo target count = %d, want 1", len(cfg.Targets.Silo))
	}
	if got := cfg.Targets.Silo[0].Verbosity; got != "debug" {
		t.Errorf("verbosity = %q, want debug", got)
	}
}

func TestConfigRejectsUnknownSiloField(t *testing.T) {
	const input = `targets:
  silo:
    - url: http://silo.test:8080
      token: test-token
      bogus: 1
`
	_, err := decodeConfig(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "field bogus not found in type silo.Config") {
		t.Fatalf("strict config decode error = %v, want unknown Silo field error", err)
	}
}
