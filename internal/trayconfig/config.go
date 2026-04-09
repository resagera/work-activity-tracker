package trayconfig

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultConfigName    = "tray-config.json"
	DefaultPollInterval  = 5 * time.Second
	DefaultRequestTimout = 3 * time.Second
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", s, err)
		}
		d.Duration = parsed
		return nil
	}

	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		d.Duration = time.Duration(n)
		return nil
	}

	return fmt.Errorf("duration must be string like \"5s\" or integer nanoseconds")
}

type Config struct {
	APIBaseURL     string   `json:"api_base_url"`
	PollInterval   Duration `json:"poll_interval"`
	RequestTimeout Duration `json:"request_timeout"`
}

func Default() Config {
	return Config{
		APIBaseURL:     "http://127.0.0.1:8080",
		PollInterval:   Duration{Duration: DefaultPollInterval},
		RequestTimeout: Duration{Duration: DefaultRequestTimout},
	}
}

func ResolvePath(explicit string) string {
	candidates := make([]string, 0, 3)
	if explicit != "" {
		candidates = append(candidates, explicit)
	} else {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, defaultConfigName))
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), defaultConfigName))
		}
		candidates = append(candidates, defaultConfigName)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}

	if cfg.PollInterval.Duration <= 0 {
		cfg.PollInterval = Duration{Duration: DefaultPollInterval}
	}
	if cfg.RequestTimeout.Duration <= 0 {
		cfg.RequestTimeout = Duration{Duration: DefaultRequestTimout}
	}
	if strings.TrimSpace(cfg.APIBaseURL) == "" {
		cfg.APIBaseURL = Default().APIBaseURL
	}

	return cfg, nil
}

func LoadFromArgs(args []string) (Config, error) {
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				return Config{}, fmt.Errorf("%s requires value", arg)
			}
			configPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "-config="):
			configPath = strings.TrimPrefix(arg, "-config=")
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		}
	}

	return Load(ResolvePath(configPath))
}

func OverrideFromFlags(cfg *Config, args []string) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		configPath     = fs.String("config", "", "path to tray-config.json")
		apiBaseURL     = fs.String("api-base-url", "", "base URL of tracker HTTP API")
		pollInterval   = fs.Duration("poll-interval", 0, "status polling interval")
		requestTimeout = fs.Duration("request-timeout", 0, "HTTP request timeout")
	)

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *configPath != "" {
		loaded, err := Load(*configPath)
		if err != nil {
			return err
		}
		*cfg = loaded
	}

	if *apiBaseURL != "" {
		cfg.APIBaseURL = *apiBaseURL
	}
	if *pollInterval > 0 {
		cfg.PollInterval = Duration{Duration: *pollInterval}
	}
	if *requestTimeout > 0 {
		cfg.RequestTimeout = Duration{Duration: *requestTimeout}
	}

	return nil
}
