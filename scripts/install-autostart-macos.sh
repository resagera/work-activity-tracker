#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
macOS autostart setup is not implemented yet.

Expected future implementation:
  - build the macOS tracker binary
  - generate a LaunchAgent plist under ~/Library/LaunchAgents
  - load it with launchctl bootstrap gui/$UID
EOF

exit 1
