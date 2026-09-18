package config

import (
	"testing"
	"time"
)

func TestEffectiveSafetyInheritsGlobalQueryTimeout(t *testing.T) {
	cfg := &Config{
		Safety: SafetyConfig{
			Mode:         "read-write",
			MaxRows:      1000,
			QueryTimeout: Duration(30 * time.Second),
		},
		DataSources: []DataSourceConfig{
			{
				Name: "main-mysql",
				Safety: &SafetyConfig{
					Mode: "read-write",
				},
			},
		},
	}
	got := cfg.EffectiveSafety("main-mysql")
	if got.QueryTimeout.Std() != 30*time.Second {
		t.Errorf("QueryTimeout = %v, want 30s (should inherit global)", got.QueryTimeout.Std())
	}
	if got.MaxRows != 1000 {
		t.Errorf("MaxRows = %d, want 1000 (should inherit global)", got.MaxRows)
	}
}

func TestEffectiveSafetyOverridesGlobal(t *testing.T) {
	cfg := &Config{
		Safety: SafetyConfig{
			Mode:         "read-write",
			MaxRows:      1000,
			QueryTimeout: Duration(30 * time.Second),
		},
		DataSources: []DataSourceConfig{
			{
				Name: "main-mysql",
				Safety: &SafetyConfig{
					Mode:         "read-only",
					MaxRows:      500,
					QueryTimeout: Duration(10 * time.Second),
				},
			},
		},
	}
	got := cfg.EffectiveSafety("main-mysql")
	if got.Mode != "read-only" {
		t.Errorf("Mode = %s, want read-only (ds override)", got.Mode)
	}
	if got.MaxRows != 500 {
		t.Errorf("MaxRows = %d, want 500 (ds override)", got.MaxRows)
	}
	if got.QueryTimeout.Std() != 10*time.Second {
		t.Errorf("QueryTimeout = %v, want 10s (ds override)", got.QueryTimeout.Std())
	}
}

func TestEffectiveSafetyNoDSConfig(t *testing.T) {
	cfg := &Config{
		Safety: SafetyConfig{
			Mode:         "read-write",
			MaxRows:      1000,
			QueryTimeout: Duration(30 * time.Second),
		},
		DataSources: []DataSourceConfig{
			{Name: "main-mysql"},
		},
	}
	got := cfg.EffectiveSafety("main-mysql")
	if got.QueryTimeout.Std() != 30*time.Second {
		t.Errorf("QueryTimeout = %v, want 30s (global fallback)", got.QueryTimeout.Std())
	}
}

func TestDocconvDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.Docconv.Workdir != "./data/docconv" {
		t.Errorf("Workdir = %q, want ./data/docconv", cfg.Docconv.Workdir)
	}
	if cfg.Docconv.MaxFileSizeMB != 50 {
		t.Errorf("MaxFileSizeMB = %d, want 50", cfg.Docconv.MaxFileSizeMB)
	}
	if cfg.Docconv.Com.ProgID != "auto" {
		t.Errorf("Com.ProgID = %q, want auto", cfg.Docconv.Com.ProgID)
	}
	if cfg.Docconv.Com.Timeout.Std() != 60*time.Second {
		t.Errorf("Com.Timeout = %v, want 60s", cfg.Docconv.Com.Timeout.Std())
	}
}

func TestDocconvExplicit(t *testing.T) {
	cfg := &Config{
		Docconv: DocconvConfig{
			Workdir:        "/data/x",
			MaxFileSizeMB:  10,
			Com: ComConfig{ProgID: "Word.Application", Timeout: Duration(120 * time.Second)},
		},
	}
	cfg.applyDefaults()
	if cfg.Docconv.Workdir != "/data/x" {
		t.Errorf("explicit Workdir overridden: %q", cfg.Docconv.Workdir)
	}
	if cfg.Docconv.Com.ProgID != "Word.Application" {
		t.Errorf("explicit ProgID overridden: %q", cfg.Docconv.Com.ProgID)
	}
}
