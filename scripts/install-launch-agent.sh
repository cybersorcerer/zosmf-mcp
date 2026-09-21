#!/bin/sh
#
# Installs a LaunchAgent that starts the zosmf-mcp container at login, so the
# server survives a reboot without anyone opening a terminal. macOS only.
#
#   scripts/install-launch-agent.sh              install and start
#   scripts/install-launch-agent.sh --uninstall  remove it again
#
# A LaunchAgent runs as the logged-in user, which is what Apple's container
# CLI expects. It therefore needs that user to log in after a reboot.

set -eu

LABEL=com.github.cybersorcerer.zosmf-mcp
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$HOME/Library/Logs/zosmf-mcp.log"

REPO=$(cd "$(dirname "$0")/.." && pwd)
DOMAIN="gui/$(id -u)"

if [ "${1:-}" = "--uninstall" ]; then
	launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
	rm -f "$PLIST"
	echo "removed $LABEL"
	exit 0
fi

if [ ! -x "$REPO/scripts/container-autostart.sh" ]; then
	echo "scripts/container-autostart.sh is not executable" >&2
	exit 1
fi

mkdir -p "$(dirname "$PLIST")" "$(dirname "$LOG")"

cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>$REPO/scripts/container-autostart.sh</string>
	</array>
	<key>WorkingDirectory</key>
	<string>$REPO</string>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>$LOG</string>
	<key>StandardErrorPath</key>
	<string>$LOG</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
EOF

# bootout first so a reinstall picks up a changed path without a logout.
launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$PLIST"

echo "installed $LABEL"
echo "  plist: $PLIST"
echo "  log:   $LOG"
echo "  start now: launchctl kickstart $DOMAIN/$LABEL"
