package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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
			Workdir:       "/data/x",
			MaxFileSizeMB: 10,
			Com:           ComConfig{ProgID: "Word.Application", Timeout: Duration(120 * time.Second)},
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

func parseDesktop(t *testing.T, yamlBody string) *Config {
	t.Helper()
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlBody), &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.applyDefaults()
	return &cfg
}

func TestDesktopDefaultsOff(t *testing.T) {
	cfg := parseDesktop(t, "server:\n  name: mcp-x\n")
	if cfg.Desktop.Enabled {
		t.Error("desktop must be disabled unless explicitly enabled")
	}
	if cfg.Desktop.Mouse.Enabled {
		t.Error("mouse.Enabled must default to false")
	}
	if cfg.Desktop.Keyboard.Enabled {
		t.Error("keyboard must default to disabled; it needs human review")
	}
	if cfg.Desktop.Mouse.AllowButtons == nil {
		t.Error("allow_buttons should get a default list")
	}
	if cfg.Desktop.PositionTolerancePx != 2 {
		t.Errorf("PositionTolerancePx = %d, want 2", cfg.Desktop.PositionTolerancePx)
	}
	if cfg.Desktop.EdgeMargin != 1 {
		t.Errorf("EdgeMargin = %d, want 1", cfg.Desktop.EdgeMargin)
	}
	if cfg.Desktop.MaxActionsPerMinute != 60 {
		t.Errorf("MaxActionsPerMinute = %d, want 60", cfg.Desktop.MaxActionsPerMinute)
	}
	if cfg.Desktop.ConfirmTTL.Std() != 60*time.Second {
		t.Errorf("ConfirmTTL = %v, want 60s", cfg.Desktop.ConfirmTTL.Std())
	}
	if !cfg.Desktop.DpiAware {
		t.Error("DpiAware must default to true; off causes coordinate drift on scaled displays")
	}
	if !cfg.Desktop.Screenshot.AllowFullScreen {
		t.Error("AllowFullScreen must default to true when unset")
	}
	if cfg.Desktop.Screenshot.Workdir != "./data/desktop" {
		t.Errorf("screenshot workdir = %q, want ./data/desktop", cfg.Desktop.Screenshot.Workdir)
	}
}

func TestDesktopExplicitFalseSurvivesDefaults(t *testing.T) {
	// A YAML `false` must win over the true default; this is why the config
	// uses pointer bools.
	cfg := parseDesktop(t, "desktop:\n  enabled: true\n  dpi_aware: false\n  screenshot:\n    enabled: true\n    allow_fullscreen: false\n")
	if !cfg.Desktop.Enabled {
		t.Error("desktop.enabled=true must be honored")
	}
	if cfg.Desktop.DpiAware {
		t.Error("explicit dpi_aware: false must not be flipped back to true")
	}
	if cfg.Desktop.Screenshot.AllowFullScreen {
		t.Error("explicit allow_fullscreen: false must not be flipped back to true")
	}
}

func TestDesktopFullConfig(t *testing.T) {
	cfg := parseDesktop(t, `
desktop:
  enabled: true
  max_actions_per_minute: 30
  position_tolerance_px: 5
  require_foreground: false
  edge_margin: 3
  confirm_required: true
  confirm_ttl: 120s
  blocked_regions:
    - x: 0
      y: 1040
      width: 1920
      height: 40
      reason: "taskbar"
  mouse:
    enabled: true
    allow_buttons: ["left"]
    allow_double_click: false
    allow_drag: true
    drag_steps: 30
    drag_duration_ms: 500
  keyboard:
    enabled: true
    require_confirm: true
    max_text_length: 100
  screenshot:
    enabled: true
    workdir: "./shots"
    max_width: 1200
    allow_fullscreen: true
`)
	d := cfg.Desktop
	if !d.Enabled || d.MaxActionsPerMinute != 30 || d.PositionTolerancePx != 5 {
		t.Errorf("top-level values not parsed: %+v", d)
	}
	if d.RequireForeground {
		t.Error("require_foreground=false not parsed")
	}
	if d.EdgeMargin != 3 || !d.ConfirmRequired || d.ConfirmTTL.Std() != 120*time.Second {
		t.Errorf("safety knobs not parsed: %+v", d)
	}
	if len(d.BlockedRegions) != 1 || d.BlockedRegions[0].Reason != "taskbar" {
		t.Errorf("blocked_regions not parsed: %+v", d.BlockedRegions)
	}
	if d.Mouse.AllowDoubleClick {
		t.Error("allow_double_click=false not parsed")
	}
	if len(d.Mouse.AllowButtons) != 1 || d.Mouse.AllowButtons[0] != "left" {
		t.Errorf("allow_buttons not parsed: %v", d.Mouse.AllowButtons)
	}
	if d.Mouse.DragSteps != 30 || d.Mouse.DragDurationMs != 500 {
		t.Errorf("drag tuning not parsed: %+v", d.Mouse)
	}
	if !d.Keyboard.Enabled || !d.Keyboard.RequireConfirm || d.Keyboard.MaxTextLength != 100 {
		t.Errorf("keyboard config not parsed: %+v", d.Keyboard)
	}
	if d.Screenshot.Workdir != "./shots" || d.Screenshot.MaxWidth != 1200 {
		t.Errorf("screenshot config not parsed: %+v", d.Screenshot)
	}
}

func TestDesktopPartialKeepsDefaults(t *testing.T) {
	cfg := parseDesktop(t, "desktop:\n  enabled: true\n  mouse:\n    enabled: true\n")
	if cfg.Desktop.Mouse.DragSteps != 20 || cfg.Desktop.Mouse.DragDurationMs != 300 {
		t.Errorf("drag defaults lost: %+v", cfg.Desktop.Mouse)
	}
	if cfg.Desktop.Screenshot.MaxWidth != 1600 {
		t.Errorf("screenshot max_width default lost: %d", cfg.Desktop.Screenshot.MaxWidth)
	}
}
