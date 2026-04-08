package linuxx11

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"work-activity-tracker/internal/platform"
)

type Environment struct{}

func New() *Environment {
	return &Environment{}
}

func (e *Environment) IdleDuration() (time.Duration, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	obj := conn.Object("org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core")
	call := obj.Call("org.gnome.Mutter.IdleMonitor.GetIdletime", 0)
	if call.Err != nil {
		return 0, call.Err
	}

	var ms uint64
	if err := dbus.Store(call.Body, &ms); err != nil {
		return 0, err
	}

	return time.Duration(ms) * time.Millisecond, nil
}

func (e *Environment) IsScreenLocked() (bool, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	obj := conn.Object("org.gnome.ScreenSaver", "/org/gnome/ScreenSaver")
	v, err := obj.GetProperty("org.gnome.ScreenSaver.Active")
	if err == nil {
		if active, ok := v.Value().(bool); ok {
			return active, nil
		}
	}

	return false, nil
}

func (e *Environment) WatchScreenLock(ctx context.Context, onChange func(bool)) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	rule := "type='signal',interface='org.gnome.ScreenSaver',member='ActiveChanged'"
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		return call.Err
	}

	signals := make(chan *dbus.Signal, 10)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)

	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-signals:
			if sig == nil {
				continue
			}
			if sig.Name != "org.gnome.ScreenSaver.ActiveChanged" {
				continue
			}
			if len(sig.Body) < 1 {
				continue
			}
			locked, ok := sig.Body[0].(bool)
			if !ok {
				continue
			}
			onChange(locked)
		}
	}
}

func (e *Environment) ActiveWindowInfo(excluded []string) (platform.WindowInfo, error) {
	windowID, err := getFocusedWindowID()
	if err != nil {
		return platform.WindowInfo{}, err
	}

	title, err := getFocusedWindowTitle()
	if err != nil {
		return platform.WindowInfo{}, err
	}

	xpropOut, err := getWindowXProp(windowID)
	if err != nil {
		return platform.WindowInfo{}, err
	}

	info := platform.WindowInfo{
		WindowID:         windowID,
		Title:            title,
		GTKApplicationID: parseXPropValue(xpropOut, "_GTK_APPLICATION_ID"),
		KDEDesktopFile:   parseXPropValue(xpropOut, "_KDE_NET_WM_DESKTOP_FILE"),
		WMClass:          parseWMClass(xpropOut),
	}

	blocked, field, substr := matchWindowInfo(info, excluded)
	info.BlockedByRule = blocked
	info.MatchedField = field
	info.MatchedSubstring = substr

	return info, nil
}

func (e *Environment) SendDesktopNotification(title, body string) error {
	cmd := exec.Command("notify-send", title, body)
	return cmd.Run()
}

func getFocusedWindowID() (string, error) {
	cmd := exec.Command("xdotool", "getwindowfocus")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getFocusedWindowTitle() (string, error) {
	cmd := exec.Command("xdotool", "getwindowfocus", "getwindowname")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getWindowXProp(windowID string) (string, error) {
	cmd := exec.Command("xprop", "-id", windowID)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return "", fmt.Errorf("%w: %s", err, errText)
		}
		return "", err
	}

	return stdout.String(), nil
}

func parseXPropValue(xpropOutput, key string) string {
	lines := strings.Split(xpropOutput, "\n")
	prefix := key + "("
	altPrefix := key + " ="

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, prefix) || strings.HasPrefix(line, altPrefix) || strings.HasPrefix(line, key+" ") {
			if idx := strings.Index(line, "="); idx >= 0 {
				v := strings.TrimSpace(line[idx+1:])
				return trimQuoted(v)
			}
		}
	}

	return ""
}

func parseWMClass(xpropOutput string) string {
	lines := strings.Split(xpropOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WM_CLASS") {
			if idx := strings.Index(line, "="); idx >= 0 {
				v := strings.TrimSpace(line[idx+1:])
				return trimQuoted(v)
			}
		}
	}
	return ""
}

func trimQuoted(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	return s
}

func matchWindowInfo(info platform.WindowInfo, excluded []string) (bool, string, string) {
	fields := []struct {
		name  string
		value string
	}{
		{name: "title", value: info.Title},
		{name: "_GTK_APPLICATION_ID", value: info.GTKApplicationID},
		{name: "_KDE_NET_WM_DESKTOP_FILE", value: info.KDEDesktopFile},
		{name: "WM_CLASS", value: info.WMClass},
	}

	for _, sub := range excluded {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}

		for _, field := range fields {
			if strings.Contains(strings.ToLower(field.value), strings.ToLower(sub)) {
				return true, field.name, sub
			}
		}
	}

	return false, "", ""
}
