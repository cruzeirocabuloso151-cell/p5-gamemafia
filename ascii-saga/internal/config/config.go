// Package config loads the ascii-saga TOML configuration.
//
// We intentionally avoid pulling in viper here. The schema is small,
// the file is short-lived per session, and a hand-written parser keeps
// the dependency surface tight. If the project grows hot-reload needs
// later, this is the place to swap in viper.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type LLMConfig struct {
	BaseURL         string
	APIKey          string
	ModelMaster     string
	ModelDirector   string
	ModelArtist     string
	ModelCompanion  string
	TimeoutSeconds  int
}

type StorageConfig struct {
	SavesDir string
	Campaign string
}

type TUIConfig struct {
	Theme         string
	ArtWidth      int
	ArtHeight     int
	TypewriterMS  int
	MinTermWidth  int
	MinTermHeight int
}

type ArtistConfig struct {
	LibraryDir                string
	AllowPalette              string
	FallbackOnValidationError bool
}

type DebugConfig struct {
	LogFile      string
	LogLLMCalls  bool
}

type Config struct {
	LLM     LLMConfig
	Storage StorageConfig
	TUI     TUIConfig
	Artist  ArtistConfig
	Debug   DebugConfig
}

// Default returns a config with sane defaults so the game can boot
// even when configs/config.toml is missing.
func Default() Config {
	return Config{
		LLM: LLMConfig{
			BaseURL:        "http://localhost:1234/v1",
			APIKey:         "lm-studio",
			ModelMaster:    "google/gemma-3-12b",
			ModelDirector:  "google/gemma-3-4b",
			ModelArtist:    "google/gemma-3-4b",
			ModelCompanion: "google/gemma-3-12b",
			TimeoutSeconds: 30,
		},
		Storage: StorageConfig{SavesDir: "./saves", Campaign: "default"},
		TUI: TUIConfig{
			Theme:         "dark_amber",
			ArtWidth:      48,
			ArtHeight:     20,
			TypewriterMS:  18,
			MinTermWidth:  100,
			MinTermHeight: 30,
		},
		Artist: ArtistConfig{
			LibraryDir:                "./internal/assets/library",
			AllowPalette:              "default",
			FallbackOnValidationError: true,
		},
		Debug: DebugConfig{LogFile: "./saves/ascii-saga.log", LogLLMCalls: true},
	}
}

// Load reads a minimal TOML file (sections + key=value, no nesting beyond one level).
// Missing file returns Default() with no error so the game still boots.
func Load(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()

	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		raw := strings.TrimSpace(line[eq+1:])
		if hash := indexUnquoted(raw, '#'); hash >= 0 {
			raw = strings.TrimSpace(raw[:hash])
		}
		applyKV(&cfg, section, key, unquote(raw))
	}
	return cfg, sc.Err()
}

func indexUnquoted(s string, c byte) int {
	inQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
		}
		if !inQuote && s[i] == c {
			return i
		}
	}
	return -1
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func applyKV(c *Config, section, key, val string) {
	switch section {
	case "llm":
		switch key {
		case "base_url":
			c.LLM.BaseURL = val
		case "api_key":
			c.LLM.APIKey = val
		case "model_master":
			c.LLM.ModelMaster = val
		case "model_director":
			c.LLM.ModelDirector = val
		case "model_artist":
			c.LLM.ModelArtist = val
		case "model_companion":
			c.LLM.ModelCompanion = val
		case "timeout_seconds":
			c.LLM.TimeoutSeconds = atoi(val, c.LLM.TimeoutSeconds)
		}
	case "storage":
		switch key {
		case "saves_dir":
			c.Storage.SavesDir = val
		case "campaign":
			c.Storage.Campaign = val
		}
	case "tui":
		switch key {
		case "theme":
			c.TUI.Theme = val
		case "art_width":
			c.TUI.ArtWidth = atoi(val, c.TUI.ArtWidth)
		case "art_height":
			c.TUI.ArtHeight = atoi(val, c.TUI.ArtHeight)
		case "typewriter_ms":
			c.TUI.TypewriterMS = atoi(val, c.TUI.TypewriterMS)
		case "min_term_width":
			c.TUI.MinTermWidth = atoi(val, c.TUI.MinTermWidth)
		case "min_term_height":
			c.TUI.MinTermHeight = atoi(val, c.TUI.MinTermHeight)
		}
	case "artist":
		switch key {
		case "library_dir":
			c.Artist.LibraryDir = val
		case "allow_palette":
			c.Artist.AllowPalette = val
		case "fallback_on_validation_error":
			c.Artist.FallbackOnValidationError = (val == "true")
		}
	case "debug":
		switch key {
		case "log_file":
			c.Debug.LogFile = val
		case "log_llm_calls":
			c.Debug.LogLLMCalls = (val == "true")
		}
	}
}

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// CampaignDir returns the folder where the current campaign's state lives.
func (c Config) CampaignDir() string {
	return filepath.Join(c.Storage.SavesDir, c.Storage.Campaign)
}

// EnsureDirs creates the saves campaign directory and any debug log parent.
func (c Config) EnsureDirs() error {
	if err := os.MkdirAll(c.CampaignDir(), 0o755); err != nil {
		return fmt.Errorf("create campaign dir: %w", err)
	}
	if c.Debug.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(c.Debug.LogFile), 0o755); err != nil {
			return fmt.Errorf("create log dir: %w", err)
		}
	}
	return nil
}
