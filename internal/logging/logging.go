package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	stdoutW io.Writer = os.Stdout
)

func Configure(logPath string) (func(), error) {
	mu.Lock()
	defer mu.Unlock()

	stdoutW = os.Stdout
	log.SetOutput(os.Stderr)

	if logPath == "" {
		return func() {}, nil
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	stdoutW = io.MultiWriter(os.Stdout, f)
	log.SetOutput(io.MultiWriter(os.Stderr, f))

	return func() {
		_ = f.Close()
	}, nil
}

func Stdoutf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = fmt.Fprintf(stdoutW, format, args...)
}
