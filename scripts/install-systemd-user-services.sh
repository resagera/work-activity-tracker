#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${REPO_ROOT}/bin"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"

TRACKER_BIN="${BIN_DIR}/work-activity-tracker"
TRAY_BIN="${BIN_DIR}/work-activity-tracker-tray"
TRACKER_CONFIG="${REPO_ROOT}/config.json"
TRAY_CONFIG="${REPO_ROOT}/tray-config.json"

GO_BIN="${GO_BIN:-}"
if [[ -z "${GO_BIN}" ]]; then
  if [[ -x "/home/resager/go/go1.25.1/bin/go" ]]; then
    GO_BIN="/home/resager/go/go1.25.1/bin/go"
  else
    GO_BIN="$(command -v go)"
  fi
fi

mkdir -p "${BIN_DIR}"
mkdir -p "${SYSTEMD_USER_DIR}"

echo "Building tracker binaries with ${GO_BIN}"
GOCACHE="${GOCACHE:-/tmp/go-build}" "${GO_BIN}" build -o "${TRACKER_BIN}" "${REPO_ROOT}/cmd/tracker-linux-x11"
GOCACHE="${GOCACHE:-/tmp/go-build}" "${GO_BIN}" build -o "${TRAY_BIN}" "${REPO_ROOT}/cmd/tracker-tray-linux-x11"

if [[ ! -f "${TRACKER_CONFIG}" ]]; then
  cp "${REPO_ROOT}/config.json.example" "${TRACKER_CONFIG}"
  echo "Created ${TRACKER_CONFIG} from example"
fi

if [[ ! -f "${TRAY_CONFIG}" ]]; then
  cp "${REPO_ROOT}/tray-config.json.example" "${TRAY_CONFIG}"
  echo "Created ${TRAY_CONFIG} from example"
fi

cat > "${SYSTEMD_USER_DIR}/work-activity-tracker.service" <<EOF
[Unit]
Description=Work Activity Tracker
After=graphical-session.target
Wants=graphical-session.target

[Service]
Type=simple
WorkingDirectory=${REPO_ROOT}
ExecStart=${TRACKER_BIN} --config=${TRACKER_CONFIG}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

cat > "${SYSTEMD_USER_DIR}/work-activity-tracker-tray.service" <<EOF
[Unit]
Description=Work Activity Tracker Tray
After=graphical-session.target work-activity-tracker.service
Wants=graphical-session.target work-activity-tracker.service

[Service]
Type=simple
WorkingDirectory=${REPO_ROOT}
ExecStart=${TRAY_BIN} --config=${TRAY_CONFIG}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now work-activity-tracker.service
systemctl --user enable --now work-activity-tracker-tray.service

echo
echo "Installed and started:"
echo "  work-activity-tracker.service"
echo "  work-activity-tracker-tray.service"
echo
echo "Useful commands:"
echo "  systemctl --user status work-activity-tracker.service"
echo "  systemctl --user status work-activity-tracker-tray.service"
echo "  journalctl --user -u work-activity-tracker.service -f"
echo "  journalctl --user -u work-activity-tracker-tray.service -f"
