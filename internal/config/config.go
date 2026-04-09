package config

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
	DefaultIdleWarnAfter = 2 * time.Minute
	DefaultStopAfterWarn = 1 * time.Minute
	DefaultPollInterval  = 5 * time.Second
	defaultConfigName    = "config.json"
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

	return fmt.Errorf("duration must be string like \"2m\" or integer nanoseconds")
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

type Config struct {
	TelegramToken            string   `json:"telegram_token"`
	TelegramChatID           int64    `json:"telegram_chat_id"`
	HTTPPort                 int      `json:"http_port"`
	AutoStartDay             bool     `json:"auto_start_day"`
	HistoryFile              string   `json:"history_file"`
	LogFile                  string   `json:"log_file"`
	IdleWarnAfter            Duration `json:"idle_warn_after"`
	StopAfterWarn            Duration `json:"stop_after_warn"`
	PollInterval             Duration `json:"poll_interval"`
	ExcludedWindowSubstrings []string `json:"excluded_window_substrings"`
	ShowVersion              bool     `json:"show_version"`
}

func Default() Config {
	return Config{
		AutoStartDay:  true,
		HistoryFile:   "session-history.json",
		IdleWarnAfter: Duration{Duration: DefaultIdleWarnAfter},
		StopAfterWarn: Duration{Duration: DefaultStopAfterWarn},
		PollInterval:  Duration{Duration: DefaultPollInterval},
		ExcludedWindowSubstrings: []string{
			"Telegram",
			"Youtube",
		},
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

	if cfg.IdleWarnAfter.Duration <= 0 {
		cfg.IdleWarnAfter = Duration{Duration: DefaultIdleWarnAfter}
	}
	if cfg.StopAfterWarn.Duration <= 0 {
		cfg.StopAfterWarn = Duration{Duration: DefaultStopAfterWarn}
	}
	if cfg.PollInterval.Duration <= 0 {
		cfg.PollInterval = Duration{Duration: DefaultPollInterval}
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
		configPath     = fs.String("config", "", "path to config.json")
		telegramToken  = fs.String("telegram-token", "", "telegram bot token")
		telegramChatID = fs.Int64("telegram-chat-id", 0, "telegram chat id")
		httpPort       = fs.Int("http-port", 0, "http api port, 0 disables api")
		historyFile    = fs.String("history-file", "", "path to session history json file")
		logFile        = fs.String("log-file", "", "path to optional log file")
		idleWarn       = fs.Duration("idle-warn-after", 0, "time without activity before warning")
		stopAfter      = fs.Duration("stop-after-warn", 0, "time after warning before stop")
		pollInterval   = fs.Duration("poll-interval", 0, "idle/lock polling interval")
		showVersion    = fs.Bool("version", false, "show version")
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

	if *telegramToken != "" {
		cfg.TelegramToken = *telegramToken
	}
	if *telegramChatID != 0 {
		cfg.TelegramChatID = *telegramChatID
	}
	if *httpPort != 0 {
		cfg.HTTPPort = *httpPort
	}
	if *historyFile != "" {
		cfg.HistoryFile = *historyFile
	}
	if *logFile != "" {
		cfg.LogFile = *logFile
	}
	if *idleWarn > 0 {
		cfg.IdleWarnAfter = Duration{Duration: *idleWarn}
	}
	if *stopAfter > 0 {
		cfg.StopAfterWarn = Duration{Duration: *stopAfter}
	}
	if *pollInterval > 0 {
		cfg.PollInterval = Duration{Duration: *pollInterval}
	}
	if *showVersion {
		cfg.ShowVersion = true
	}

	return nil
}
