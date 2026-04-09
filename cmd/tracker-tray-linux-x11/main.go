package main

import (
	"log"
	"os"

	"work-activity-tracker/internal/trayapp"
	"work-activity-tracker/internal/trayconfig"
)

func main() {
	cfg, err := trayconfig.LoadFromArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("load tray config: %v", err)
	}

	if err := trayconfig.OverrideFromFlags(&cfg, os.Args); err != nil {
		log.Fatalf("parse tray flags: %v", err)
	}

	if err := trayapp.PrepareRuntimeEnv(); err != nil {
		log.Fatalf("prepare tray runtime env: %v", err)
	}

	trayapp.New(cfg).Run()
}
