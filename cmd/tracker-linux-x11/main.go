package main

import (
	"work-activity-tracker/internal/bootstrap"
	"work-activity-tracker/internal/platform/linuxx11"
)

func main() {
	bootstrap.Run(linuxx11.New())
}
