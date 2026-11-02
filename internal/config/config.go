package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig       `yaml:"server"`
	Safety      SafetyConfig       `yaml:"safety"`
	DataSources []DataSourceConfig `yaml:"datasources"`
	Docconv     DocconvConfig      `yaml:"docconv"`
	Desktop     DesktopConfig      `yaml:"desktop"`
}

// desktopConfigWire mirrors DesktopConfig with pointer bools so an explicit
// `false` in YAML can be told apart from an omitted key. DpiAware and
// Screenshot.AllowFullScreen default to true, so they need this distinction.
type desktopConfigWire struct {
	Enabled             bool               `yaml:"enabled"`
	MaxActionsPerMinute int                `yaml:"max_actions_per_minute"`
	PositionTolerancePx int                `yaml:"position_tolerance_px"`
	RequireForeground   bool               `yaml:"require_foreground"`
	EdgeMargin          int                `yaml:"edge_margin"`
	ConfirmRequired     bool               `yaml:"confirm_required"`
	ConfirmTTL          Duration           `yaml:"confirm_ttl"`
	DpiAware            *bool              `yaml:"dpi_aware"`
	BlockedRegions      []DesktopRegion    `yaml:"blocked_regions"`
	Mouse               DesktopMouseCfg    `yaml:"mouse"`
	Keyboard            DesktopKeyboardCfg `yaml:"keyboard"`
	Screenshot          DesktopShotCfg     `yaml:"screenshot"`
}

type desktopShotCfgWire struct {
	Enabled         bool   `yaml:"enabled"`
	Workdir         string `yaml:"workdir"`
	MaxWidth        int    `yaml:"max_width"`
	AllowFullScreen *bool  `yaml:"allow_fullscreen"`
}

func (s *DesktopShotCfg) UnmarshalYAML(value *yaml.Node) error {
	var wire desktopShotCfgWire
	if err := value.Decode(&wire); err != nil {
		return err
	}
	s.Enabled = wire.Enabled
	s.Workdir = wire.Workdir
	s.MaxWidth = wire.MaxWidth
	s.AllowFullScreen = wire.AllowFullScreen == nil || *wire.AllowFullScreen
	s.fullScreenSeen = wire.AllowFullScreen != nil
	return nil
}

func (d *DesktopConfig) UnmarshalYAML(value *yaml.Node) error {
	var wire desktopConfigWire
	if err := value.Decode(&wire); err != nil {
		return err
	}
	d.Enabled = wire.Enabled
	d.MaxActionsPerMinute = wire.MaxActionsPerMinute
	d.PositionTolerancePx = wire.PositionTolerancePx
	d.RequireForeground = wire.RequireForeground
	d.EdgeMargin = wire.EdgeMargin
	d.ConfirmRequired = wire.ConfirmRequired
	d.ConfirmTTL = wire.ConfirmTTL
	// DpiAware and AllowFullScreen default to true, and an explicit `false`
	// in YAML has to win. applyDefaults fills in the absent case, because it
	// also runs when the whole `desktop:` block is missing.
	d.DpiAware = wire.DpiAware == nil || *wire.DpiAware
	d.dpiAwareSeen = wire.DpiAware != nil
	d.BlockedRegions = wire.BlockedRegions
	d.Mouse = wire.Mouse
	d.Keyboard = wire.Keyboard
	d.Screenshot = wire.Screenshot
	return nil
}

// DesktopConfig controls the Windows desktop control tools. Every sub-capability
// is opt-in; the whole module stays off unless explicitly enabled.
type DesktopConfig struct {
	Enabled             bool               `yaml:"enabled"`
	MaxActionsPerMinute int                `yaml:"max_actions_per_minute"`
	PositionTolerancePx int                `yaml:"position_tolerance_px"`
	RequireForeground   bool               `yaml:"require_foreground"`
	EdgeMargin          int                `yaml:"edge_margin"`
	ConfirmRequired     bool               `yaml:"confirm_required"`
	ConfirmTTL          Duration           `yaml:"confirm_ttl"`
	DpiAware            bool               `yaml:"dpi_aware"`
	BlockedRegions      []DesktopRegion    `yaml:"blocked_regions"`
	Mouse               DesktopMouseCfg    `yaml:"mouse"`
	Keyboard            DesktopKeyboardCfg `yaml:"keyboard"`
	Screenshot          DesktopShotCfg     `yaml:"screenshot"`

	// Records whether the key appeared in YAML, so applyDefaults can tell an
	// omitted `dpi_aware` from an explicit `dpi_aware: false`. Both default to
	// true, which a plain bool field cannot express.
	dpiAwareSeen   bool
	fullScreenSeen bool
}

// DesktopRegion is a screen rectangle the tools refuse to touch.
type DesktopRegion struct {
	X      int    `yaml:"x"`
	Y      int    `yaml:"y"`
	Width  int    `yaml:"width"`
	Height int    `yaml:"height"`
	Reason string `yaml:"reason"`
}

type DesktopMouseCfg struct {
	Enabled          bool     `yaml:"enabled"`
	AllowButtons     []string `yaml:"allow_buttons"`
	AllowDoubleClick bool     `yaml:"allow_double_click"`
	AllowDrag        bool     `yaml:"allow_drag"`
	DragSteps        int      `yaml:"drag_steps"`
	DragDurationMs   int      `yaml:"drag_duration_ms"`
}

type DesktopKeyboardCfg struct {
	Enabled        bool `yaml:"enabled"`
	RequireConfirm bool `yaml:"require_confirm"`
	MaxTextLength  int  `yaml:"max_text_length"`
}

type DesktopShotCfg struct {
	Enabled         bool   `yaml:"enabled"`
	Workdir         string `yaml:"workdir"`
	MaxWidth        int    `yaml:"max_width"`
	AllowFullScreen bool   `yaml:"allow_fullscreen"`

	fullScreenSeen bool
}

type DocconvConfig struct {
	Enabled       bool      `yaml:"enabled"`
	Workdir       string    `yaml:"workdir"`
	MaxFileSizeMB int       `yaml:"max_file_size_mb"`
	Com           ComConfig `yaml:"com"`
}

type ComConfig struct {
	ProgID  string   `yaml:"prog_id"`
	Timeout Duration `yaml:"timeout"`
}

type ServerConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type SafetyConfig struct {
	Mode                 string   `yaml:"mode"`
	MaxRows              int      `yaml:"max_rows"`
	QueryTimeout         Duration `yaml:"query_timeout"`
	BlockedKeywords      []string `yaml:"blocked_keywords"`
	AllowBlockedKeywords []string `yaml:"allow_blocked_keywords"`
	BlockedCommands      []string `yaml:"blocked_commands"`
	AllowBlockedCommands []string `yaml:"allow_blocked_commands"`
}

type DataSourceConfig struct {
	Name            string        `yaml:"name"`
	Driver          string        `yaml:"driver"`
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration      `yaml:"conn_max_lifetime"`
	Addr            string        `yaml:"addr"`
	Addrs           []string      `yaml:"addrs"`
	Password        string        `yaml:"password"`
	DB              int           `yaml:"db"`
	Mode            string        `yaml:"mode"`
	PoolSize        int           `yaml:"pool_size"`
	Safety          *SafetyConfig `yaml:"safety"`
	Username        string        `yaml:"username"`
	Endpoint        string        `yaml:"endpoint"`
	Endpoints       []string      `yaml:"endpoints"`
	AccessKey       string        `yaml:"access_key"`
	SecretKey       string        `yaml:"secret_key"`
	UseSSL          bool          `yaml:"use_ssl"`
	Region          string        `yaml:"region"`
	Bucket          string        `yaml:"bucket"`
	IndexName       string        `yaml:"index_name"`
	Database        string        `yaml:"database"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Safety.Mode == "" {
		c.Safety.Mode = "read-write"
	}
	if c.Safety.MaxRows == 0 {
		c.Safety.MaxRows = 1000
	}
	if c.Safety.QueryTimeout == 0 {
		c.Safety.QueryTimeout = Duration(30 * time.Second)
	}
	if c.Safety.BlockedKeywords == nil {
		c.Safety.BlockedKeywords = []string{"DROP", "TRUNCATE", "GRANT", "REVOKE", "ALTER", "SHUTDOWN"}
	}
	if c.Safety.BlockedCommands == nil {
		c.Safety.BlockedCommands = []string{"FLUSHALL", "FLUSHDB", "CONFIG", "SHUTDOWN", "KEYS", "BGREWRITEAOF", "BGSAVE"}
	}
	if c.Docconv.Workdir == "" {
		c.Docconv.Workdir = "./data/docconv"
	}
	if c.Docconv.MaxFileSizeMB == 0 {
		c.Docconv.MaxFileSizeMB = 50
	}
	if c.Docconv.Com.ProgID == "" {
		c.Docconv.Com.ProgID = "auto"
	}
	if c.Docconv.Com.Timeout == 0 {
		c.Docconv.Com.Timeout = Duration(60 * time.Second)
	}
	c.Desktop.applyDefaults()
	for i := range c.DataSources {
		ds := &c.DataSources[i]
		if ds.Mode == "" {
			ds.Mode = "standalone"
		}
		if ds.MaxOpenConns == 0 {
			ds.MaxOpenConns = 10
		}
		if ds.MaxIdleConns == 0 {
			ds.MaxIdleConns = 5
		}
		if ds.ConnMaxLifetime == 0 {
			ds.ConnMaxLifetime = Duration(5 * time.Minute)
		}
		if ds.PoolSize == 0 {
			ds.PoolSize = 10
		}
	}
}

func (d *DesktopConfig) applyDefaults() {
	// DpiAware and AllowFullScreen default to true, so the default can only be
	// applied when the key was absent from YAML. Without the seen flags, an
	// explicit `false` would be silently flipped back on.
	if !d.dpiAwareSeen {
		d.DpiAware = true
	}
	if !d.Screenshot.fullScreenSeen {
		d.Screenshot.AllowFullScreen = true
	}
	if d.MaxActionsPerMinute == 0 {
		d.MaxActionsPerMinute = 60
	}
	if d.PositionTolerancePx == 0 {
		d.PositionTolerancePx = 2
	}
	if d.EdgeMargin == 0 {
		d.EdgeMargin = 1
	}
	if d.ConfirmTTL == 0 {
		d.ConfirmTTL = Duration(60 * time.Second)
	}
	if d.Mouse.AllowButtons == nil {
		d.Mouse.AllowButtons = []string{"left", "right", "middle"}
	}
	if d.Mouse.DragSteps == 0 {
		d.Mouse.DragSteps = 20
	}
	if d.Mouse.DragDurationMs == 0 {
		d.Mouse.DragDurationMs = 300
	}
	if d.Keyboard.MaxTextLength == 0 {
		d.Keyboard.MaxTextLength = 2000
	}
	if d.Screenshot.Workdir == "" {
		d.Screenshot.Workdir = "./data/desktop"
	}
	if d.Screenshot.MaxWidth == 0 {
		d.Screenshot.MaxWidth = 1600
	}
}

func (c *Config) EffectiveSafety(dsName string) SafetyConfig {
	merged := c.Safety
	for _, ds := range c.DataSources {
		if ds.Name != dsName || ds.Safety == nil {
			continue
		}
		ds := *ds.Safety
		if ds.Mode != "" {
			merged.Mode = ds.Mode
		}
		if ds.MaxRows != 0 {
			merged.MaxRows = ds.MaxRows
		}
		if ds.QueryTimeout != 0 {
			merged.QueryTimeout = ds.QueryTimeout
		}
		if ds.BlockedKeywords != nil {
			merged.BlockedKeywords = ds.BlockedKeywords
		}
		if ds.AllowBlockedKeywords != nil {
			merged.AllowBlockedKeywords = ds.AllowBlockedKeywords
		}
		if ds.BlockedCommands != nil {
			merged.BlockedCommands = ds.BlockedCommands
		}
		if ds.AllowBlockedCommands != nil {
			merged.AllowBlockedCommands = ds.AllowBlockedCommands
		}
	}
	return merged
}
