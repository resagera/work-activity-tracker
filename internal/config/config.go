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

type ExcludedRule struct {
	Tag     string        `json:"tag"`
	Type    string        `json:"type"`
	Exclude *ExcludedRule `json:"exclude,omitempty"`
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
	TelegramToken                 string         `json:"telegram_token"`
	TelegramChatID                int64          `json:"telegram_chat_id"`
	TelegramControlsOnly          bool           `json:"telegram_controls_only"`
	HTTPPort                      int            `json:"http_port"`
	AutoStartDay                  bool           `json:"auto_start_day"`
	EnableDesktopNotifications    bool           `json:"enable_desktop_notifications"`
	HistoryFile                   string         `json:"history_file"`
	ActivityTypesFile             string         `json:"activity_types_file"`
	DefaultActivityType           string         `json:"default_activity_type"`
	InactivityTypesFile           string         `json:"inactivity_types_file"`
	LogFile                       string         `json:"log_file"`
	IdleWarnAfter                 Duration       `json:"idle_warn_after"`
	StopAfterWarn                 Duration       `json:"stop_after_warn"`
	PollInterval                  Duration       `json:"poll_interval"`
	Excluded                      []ExcludedRule `json:"excluded"`
	ExcludedWindowSubstrings      []string       `json:"excluded_window_substrings"`
	ExcludedWindowTitleSubstrings []string       `json:"excluded_window_title_substrings"`
	ExcludedAppSubstrings         []string       `json:"excluded_app_substrings"`
	ShowVersion                   bool           `json:"show_version"`
}

func Default() Config {
	return Config{
		AutoStartDay:               true,
		EnableDesktopNotifications: true,
		HistoryFile:                "session-history.json",
		ActivityTypesFile:          "activity-types.json",
		DefaultActivityType:        "работа",
		InactivityTypesFile:        "inactivity-types.json",
		IdleWarnAfter:              Duration{Duration: DefaultIdleWarnAfter},
		StopAfterWarn:              Duration{Duration: DefaultStopAfterWarn},
		PollInterval:               Duration{Duration: DefaultPollInterval},
		ExcludedWindowSubstrings: []string{
			"Telegram",
			"Youtube",
		},
		ExcludedWindowTitleSubstrings: []string{"Telegram", "Youtube"},
		Excluded: []ExcludedRule{
			{Tag: "Telegram", Type: "title"},
			{Tag: "Youtube", Type: "title"},
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
	if len(cfg.ExcludedWindowTitleSubstrings) == 0 && len(cfg.ExcludedWindowSubstrings) > 0 {
		cfg.ExcludedWindowTitleSubstrings = append([]string{}, cfg.ExcludedWindowSubstrings...)
	}
	if len(cfg.Excluded) == 0 {
		cfg.Excluded = legacyExcludedRules(cfg.ExcludedWindowTitleSubstrings, cfg.ExcludedAppSubstrings)
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
		configPath                 = fs.String("config", "", "path to config.json")
		telegramToken              = fs.String("telegram-token", "", "telegram bot token")
		telegramChatID             = fs.Int64("telegram-chat-id", 0, "telegram chat id")
		httpPort                   = fs.Int("http-port", 0, "http api port, 0 disables api")
		enableDesktopNotifications = fs.Bool("enable-desktop-notifications", true, "enable desktop notifications")
		historyFile                = fs.String("history-file", "", "path to session history json file")
		activityTypesFile          = fs.String("activity-types-file", "", "path to activity types json file")
		defaultActivityType        = fs.String("default-activity-type", "", "default activity type for new day")
		inactivityTypesFile        = fs.String("inactivity-types-file", "", "path to inactivity types json file")
		logFile                    = fs.String("log-file", "", "path to optional log file")
		idleWarn                   = fs.Duration("idle-warn-after", 0, "time without activity before warning")
		stopAfter                  = fs.Duration("stop-after-warn", 0, "time after warning before stop")
		pollInterval               = fs.Duration("poll-interval", 0, "idle/lock polling interval")
		showVersion                = fs.Bool("version", false, "show version")
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
	cfg.EnableDesktopNotifications = *enableDesktopNotifications
	if *historyFile != "" {
		cfg.HistoryFile = *historyFile
	}
	if *activityTypesFile != "" {
		cfg.ActivityTypesFile = *activityTypesFile
	}
	if *defaultActivityType != "" {
		cfg.DefaultActivityType = *defaultActivityType
	}
	if *inactivityTypesFile != "" {
		cfg.InactivityTypesFile = *inactivityTypesFile
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

func legacyExcludedRules(titleExcluded, appExcluded []string) []ExcludedRule {
	items := make([]ExcludedRule, 0, len(titleExcluded)+len(appExcluded))
	for _, item := range titleExcluded {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, ExcludedRule{Tag: item, Type: "title"})
	}
	for _, item := range appExcluded {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, ExcludedRule{Tag: item, Type: "app"})
	}
	return items
}
