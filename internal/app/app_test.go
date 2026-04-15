package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"work-activity-tracker/internal/activity"
	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/platform"
	"work-activity-tracker/internal/tracker"
)

type nopEnv struct{}

func (nopEnv) IdleDuration() (time.Duration, error) { return 0, nil }
func (nopEnv) IsScreenLocked() (bool, error)        { return false, nil }
func (nopEnv) WatchScreenLock(context.Context, func(bool)) error {
	return nil
}
func (nopEnv) ActiveWindowInfo([]config.ExcludedRule) (platform.WindowInfo, error) {
	return platform.WindowInfo{}, nil
}
func (nopEnv) SendDesktopNotification(string, string) error { return nil }

func newTestApp(t *testing.T) *App {
	t.Helper()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.HistoryFile = filepath.Join(dir, "history.json")
	cfg.ActivityTypesFile = filepath.Join(dir, "activity-types.json")
	cfg.InactivityTypesFile = filepath.Join(dir, "inactivity-types.json")

	a := New(cfg, nopEnv{})
	if err := a.loadActivityTypes(); err != nil {
		t.Fatalf("load activity types: %v", err)
	}
	if err := a.loadInactivityTypes(); err != nil {
		t.Fatalf("load inactivity types: %v", err)
	}
	return a
}

func performRequest(t *testing.T, handler http.Handler, method, target string, dst any) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if dst != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
			t.Fatalf("decode response for %s %s: %v; body=%s", method, target, err, rec.Body.String())
		}
	}

	return rec
}

func TestHTTPHandlerRootAndStatus(t *testing.T) {
	a := newTestApp(t)
	handler := a.httpHandler()

	root := performRequest(t, handler, http.MethodGet, "/", nil)
	if root.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", root.Code, http.StatusOK)
	}
	if got := root.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("root content-type = %q, want text/html", got)
	}

	missing := performRequest(t, handler, http.MethodGet, "/missing", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", missing.Code, http.StatusNotFound)
	}

	var summary tracker.SessionSummary
	status := performRequest(t, handler, http.MethodGet, "/status", &summary)
	if status.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", status.Code, http.StatusOK)
	}
	if summary.Started {
		t.Fatalf("summary.Started = true, want false")
	}
	if summary.CurrentActivityType != "" {
		t.Fatalf("current activity type = %q, want empty value before session start", summary.CurrentActivityType)
	}
	if summary.CurrentActivityColor != "" {
		t.Fatalf("current activity color = %q, want empty value before session start", summary.CurrentActivityColor)
	}
}

func TestHTTPHandlerAddValidationAndMutation(t *testing.T) {
	a := newTestApp(t)
	handler := a.httpHandler()

	var started tracker.SessionSummary
	start := performRequest(t, handler, http.MethodPost, "/new-day", &started)
	if start.Code != http.StatusOK {
		t.Fatalf("new-day code = %d, want %d", start.Code, http.StatusOK)
	}
	if !started.Started {
		t.Fatalf("new-day did not start session")
	}

	var missingMinutes map[string]string
	badReq := performRequest(t, handler, http.MethodGet, "/add", &missingMinutes)
	if badReq.Code != http.StatusBadRequest {
		t.Fatalf("add without minutes code = %d, want %d", badReq.Code, http.StatusBadRequest)
	}
	if missingMinutes["error"] != "minutes is required" {
		t.Fatalf("unexpected add error: %q", missingMinutes["error"])
	}

	var invalidMinutes map[string]string
	invalidReq := performRequest(t, handler, http.MethodGet, "/add?minutes=-5", &invalidMinutes)
	if invalidReq.Code != http.StatusBadRequest {
		t.Fatalf("add invalid code = %d, want %d", invalidReq.Code, http.StatusBadRequest)
	}
	if invalidMinutes["error"] != "minutes must be positive integer" {
		t.Fatalf("unexpected invalid add error: %q", invalidMinutes["error"])
	}

	var updated tracker.SessionSummary
	okReq := performRequest(t, handler, http.MethodGet, "/add?minutes=15", &updated)
	if okReq.Code != http.StatusOK {
		t.Fatalf("add code = %d, want %d", okReq.Code, http.StatusOK)
	}
	if updated.TotalAdded != 15*time.Minute {
		t.Fatalf("total added = %s, want %s", updated.TotalAdded, 15*time.Minute)
	}

	var subtractErr map[string]string
	subtract := performRequest(t, handler, http.MethodGet, "/subtract?minutes=0", &subtractErr)
	if subtract.Code != http.StatusBadRequest {
		t.Fatalf("subtract invalid code = %d, want %d", subtract.Code, http.StatusBadRequest)
	}
	if subtractErr["error"] != "minutes must be positive integer" {
		t.Fatalf("unexpected subtract error: %q", subtractErr["error"])
	}
}

func TestHTTPHandlerActivityTypeLifecycle(t *testing.T) {
	a := newTestApp(t)
	handler := a.httpHandler()

	performRequest(t, handler, http.MethodPost, "/new-day", nil)

	var added struct {
		Types []activity.TypeDefinition `json:"types"`
	}
	addReq := performRequest(t, handler, http.MethodPost, "/activity-types/add?name=focus&color=%23112233", &added)
	if addReq.Code != http.StatusOK {
		t.Fatalf("activity-types/add code = %d, want %d", addReq.Code, http.StatusOK)
	}
	if !hasActivityType(added.Types, "focus", "#112233") {
		t.Fatalf("added types do not include focus with color: %#v", added.Types)
	}

	var summary tracker.SessionSummary
	setReq := performRequest(t, handler, http.MethodPost, "/activity-type/set?name=focus", &summary)
	if setReq.Code != http.StatusOK {
		t.Fatalf("activity-type/set code = %d, want %d", setReq.Code, http.StatusOK)
	}
	if summary.CurrentActivityType != "focus" {
		t.Fatalf("current activity type = %q, want %q", summary.CurrentActivityType, "focus")
	}
	if summary.CurrentActivityColor != "#112233" {
		t.Fatalf("current activity color = %q, want %q", summary.CurrentActivityColor, "#112233")
	}

	var recolored struct {
		Types []activity.TypeDefinition `json:"types"`
	}
	colorReq := performRequest(t, handler, http.MethodPost, "/activity-type/color?name=focus&color=%23445566", &recolored)
	if colorReq.Code != http.StatusOK {
		t.Fatalf("activity-type/color code = %d, want %d", colorReq.Code, http.StatusOK)
	}
	if !hasActivityType(recolored.Types, "focus", "#445566") {
		t.Fatalf("recolored types do not include focus with updated color: %#v", recolored.Types)
	}

	var unknownErr map[string]string
	unknownReq := performRequest(t, handler, http.MethodPost, "/activity-type/set?name=unknown", &unknownErr)
	if unknownReq.Code != http.StatusBadRequest {
		t.Fatalf("unknown activity type code = %d, want %d", unknownReq.Code, http.StatusBadRequest)
	}
	if !strings.Contains(unknownErr["error"], "unknown activity type") {
		t.Fatalf("unexpected unknown activity type error: %q", unknownErr["error"])
	}
}

func TestHTTPHandlerHistoryRenameFlow(t *testing.T) {
	a := newTestApp(t)
	handler := a.httpHandler()

	var missingStartedAt map[string]string
	missingReq := performRequest(t, handler, http.MethodPost, "/history/session-name?name=renamed", &missingStartedAt)
	if missingReq.Code != http.StatusBadRequest {
		t.Fatalf("missing started_at code = %d, want %d", missingReq.Code, http.StatusBadRequest)
	}
	if missingStartedAt["error"] != "started_at is required" {
		t.Fatalf("unexpected missing started_at error: %q", missingStartedAt["error"])
	}

	var invalidStartedAt map[string]string
	invalidReq := performRequest(t, handler, http.MethodPost, "/history/session-name?started_at=bad&name=renamed", &invalidStartedAt)
	if invalidReq.Code != http.StatusBadRequest {
		t.Fatalf("invalid started_at code = %d, want %d", invalidReq.Code, http.StatusBadRequest)
	}
	if invalidStartedAt["error"] != "started_at must be RFC3339" {
		t.Fatalf("unexpected invalid started_at error: %q", invalidStartedAt["error"])
	}

	startedAt := time.Date(2026, time.April, 14, 9, 30, 0, 0, time.UTC)
	notFoundURL := "/history/session-name?started_at=" + startedAt.Format(time.RFC3339Nano) + "&name=renamed"
	var notFoundErr map[string]string
	notFoundReq := performRequest(t, handler, http.MethodPost, notFoundURL, &notFoundErr)
	if notFoundReq.Code != http.StatusNotFound {
		t.Fatalf("missing history record code = %d, want %d", notFoundReq.Code, http.StatusNotFound)
	}

	var started tracker.SessionSummary
	performRequest(t, handler, http.MethodPost, "/new-day", &started)

	var setName tracker.SessionSummary
	nameReq := performRequest(t, handler, http.MethodPost, "/session-name?name=alpha", &setName)
	if nameReq.Code != http.StatusOK {
		t.Fatalf("session-name code = %d, want %d", nameReq.Code, http.StatusOK)
	}
	if setName.SessionName != "alpha" {
		t.Fatalf("session name = %q, want %q", setName.SessionName, "alpha")
	}

	performRequest(t, handler, http.MethodPost, "/end", nil)

	var historyBefore []struct {
		SessionName      string    `json:"session_name"`
		SessionStartedAt time.Time `json:"session_started_at"`
	}
	historyReq := performRequest(t, handler, http.MethodGet, "/history", &historyBefore)
	if historyReq.Code != http.StatusOK {
		t.Fatalf("history code = %d, want %d", historyReq.Code, http.StatusOK)
	}
	if len(historyBefore) != 1 {
		t.Fatalf("history length = %d, want 1", len(historyBefore))
	}
	if historyBefore[0].SessionName != "alpha" {
		t.Fatalf("history session name = %q, want %q", historyBefore[0].SessionName, "alpha")
	}

	records, err := a.history.LoadAll()
	if err != nil {
		t.Fatalf("load history records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("history store length = %d, want 1", len(records))
	}

	renameURL := "/history/session-name?started_at=" + url.QueryEscape(records[0].SessionStartedAt.Format(time.RFC3339Nano)) + "&name=beta"
	var renamed map[string]string
	renameReq := performRequest(t, handler, http.MethodPost, renameURL, &renamed)
	if renameReq.Code != http.StatusOK {
		t.Fatalf("history rename code = %d, want %d", renameReq.Code, http.StatusOK)
	}
	if renamed["ok"] != "true" {
		t.Fatalf("rename response ok = %q, want %q", renamed["ok"], "true")
	}

	var historyAfter []struct {
		SessionName string `json:"session_name"`
	}
	performRequest(t, handler, http.MethodGet, "/history", &historyAfter)
	if historyAfter[0].SessionName != "beta" {
		t.Fatalf("history session name after rename = %q, want %q", historyAfter[0].SessionName, "beta")
	}
}

func hasActivityType(items []activity.TypeDefinition, name, color string) bool {
	for _, item := range items {
		if item.Name == name && item.Color == color {
			return true
		}
	}
	return false
}
