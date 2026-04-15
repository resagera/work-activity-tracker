#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

make -C "${REPO_ROOT}" install-linux-user-services

echo
echo "Installed and started:"
echo "  work-activity-tracker.service"
echo "  work-activity-tracker-tray.service"
echo
echo "Useful commands:"
echo "  make status-linux-user-services"
echo "  make logs-linux-user-services"
echo "  systemctl --user status work-activity-tracker.service"
echo "  systemctl --user status work-activity-tracker-tray.service"
