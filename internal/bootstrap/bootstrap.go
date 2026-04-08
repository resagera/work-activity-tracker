package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"work-activity-tracker/internal/app"
	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/platform"
	"work-activity-tracker/pkg/version"
)

func Run(env platform.Environment) {
	version.MajorVersion = "1"
	version.MinorVersion = "1"

	cfg, err := config.LoadFromArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := config.OverrideFromFlags(&cfg, os.Args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	fmt.Println("Version: " + version.Get().SemVer())
	if cfg.ShowVersion {
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	instance := app.New(cfg, env)
	if err := instance.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func Unsupported(target string) {
	version.MajorVersion = "1"
	version.MinorVersion = "1"

	fmt.Println("Version: " + version.Get().SemVer())
	log.Fatalf("%s is not implemented yet", target)
}
