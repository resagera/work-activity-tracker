GO_BIN ?= /usr/bin/go
GOCACHE ?= /tmp/go-build-cache

BIN_DIR ?= $(CURDIR)/bin
SYSTEMD_USER_DIR ?= $(HOME)/.config/systemd/user

TRACKER_BIN := $(BIN_DIR)/work-activity-tracker
TRAY_BIN := $(BIN_DIR)/work-activity-tracker-tray
TRACKER_CONFIG ?= $(CURDIR)/config.json
TRAY_CONFIG := $(CURDIR)/tray-config.json

TRACKER_SERVICE := work-activity-tracker.service
TRAY_SERVICE := work-activity-tracker-tray.service

.PHONY: all build build-linux-x11 build-tray-linux-x11 configs test
.PHONY: install-linux-user-services update-linux-user-services install-systemd-user-units
.PHONY: restart-linux-user-services stop-linux-user-services status-linux-user-services logs-linux-user-services
.PHONY: install-autostart-macos install-autostart-windows

all: build

build: build-linux-x11 build-tray-linux-x11

build-linux-x11:
	mkdir -p "$(BIN_DIR)"
	GOCACHE="$(GOCACHE)" "$(GO_BIN)" build -o "$(TRACKER_BIN)" "$(CURDIR)/cmd/tracker-linux-x11"

build-tray-linux-x11:
	mkdir -p "$(BIN_DIR)"
	GOCACHE="$(GOCACHE)" "$(GO_BIN)" build -o "$(TRAY_BIN)" "$(CURDIR)/cmd/tracker-tray-linux-x11"

configs:
	test -f "$(TRACKER_CONFIG)" || cp "$(CURDIR)/config.json.example" "$(TRACKER_CONFIG)"
	test -f "$(TRAY_CONFIG)" || cp "$(CURDIR)/tray-config.json.example" "$(TRAY_CONFIG)"

test:
	GOCACHE="$(GOCACHE)" "$(GO_BIN)" test ./...

install-linux-user-services: build configs install-systemd-user-units
	systemctl --user enable --now "$(TRACKER_SERVICE)"
	systemctl --user enable --now "$(TRAY_SERVICE)"

update-linux-user-services: build install-systemd-user-units
	systemctl --user restart "$(TRACKER_SERVICE)"
	systemctl --user restart "$(TRAY_SERVICE)"

install-systemd-user-units:
	mkdir -p "$(SYSTEMD_USER_DIR)"
	sed \
		-e 's|{{WORKING_DIR}}|$(CURDIR)|g' \
		-e 's|{{TRACKER_BIN}}|$(TRACKER_BIN)|g' \
		-e 's|{{TRACKER_CONFIG}}|$(TRACKER_CONFIG)|g' \
		"$(CURDIR)/deploy/systemd/user/$(TRACKER_SERVICE)" > "$(SYSTEMD_USER_DIR)/$(TRACKER_SERVICE)"
	sed \
		-e 's|{{WORKING_DIR}}|$(CURDIR)|g' \
		-e 's|{{TRAY_BIN}}|$(TRAY_BIN)|g' \
		-e 's|{{TRAY_CONFIG}}|$(TRAY_CONFIG)|g' \
		"$(CURDIR)/deploy/systemd/user/$(TRAY_SERVICE)" > "$(SYSTEMD_USER_DIR)/$(TRAY_SERVICE)"
	systemctl --user daemon-reload

restart-linux-user-services:
	systemctl --user restart "$(TRACKER_SERVICE)"
	systemctl --user restart "$(TRAY_SERVICE)"

stop-linux-user-services:
	systemctl --user stop "$(TRACKER_SERVICE)"
	systemctl --user stop "$(TRAY_SERVICE)"

status-linux-user-services:
	systemctl --user status "$(TRACKER_SERVICE)" --no-pager
	systemctl --user status "$(TRAY_SERVICE)" --no-pager

logs-linux-user-services:
	journalctl --user -u "$(TRACKER_SERVICE)" -u "$(TRAY_SERVICE)" -f

install-autostart-macos:
	"$(CURDIR)/scripts/install-autostart-macos.sh"

install-autostart-windows:
	pwsh -NoProfile -ExecutionPolicy Bypass -File "$(CURDIR)/scripts/install-autostart-windows.ps1"
