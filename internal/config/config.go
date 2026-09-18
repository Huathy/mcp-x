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
