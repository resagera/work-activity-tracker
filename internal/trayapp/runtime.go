package trayapp

import (
	"os"
	"path/filepath"
)

func PrepareRuntimeEnv() error {
	if os.Getenv("TMPDIR") != "" && !isSnapTmp(os.Getenv("TMPDIR")) {
		return nil
	}

	candidates := []string{
		os.Getenv("XDG_RUNTIME_DIR"),
		os.Getenv("SNAP_USER_COMMON"),
		os.Getenv("SNAP_USER_DATA"),
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {
		candidates = append(candidates, filepath.Join(cacheDir, "work-activity-tracker", "tray"))
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			continue
		}
		if err := os.Setenv("TMPDIR", candidate); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func isSnapTmp(path string) bool {
	return path == "/tmp" || path == "/var/tmp"
}
