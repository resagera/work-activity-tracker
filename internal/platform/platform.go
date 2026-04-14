package platform

import (
	"context"
	"time"

	"work-activity-tracker/internal/config"
)

type WindowInfo struct {
	WindowID         string `json:"window_id"`
	Title            string `json:"title"`
	GTKApplicationID string `json:"gtk_application_id"`
	KDEDesktopFile   string `json:"kde_net_wm_desktop_file"`
	WMClass          string `json:"wm_class"`
	MatchedField     string `json:"matched_field"`
	MatchedSubstring string `json:"matched_substring"`
	BlockedByRule    bool   `json:"blocked_by_rule"`
}

type Environment interface {
	IdleDuration() (time.Duration, error)
	IsScreenLocked() (bool, error)
	WatchScreenLock(ctx context.Context, onChange func(bool)) error
	ActiveWindowInfo(excluded []config.ExcludedRule) (WindowInfo, error)
	SendDesktopNotification(title, body string) error
}
