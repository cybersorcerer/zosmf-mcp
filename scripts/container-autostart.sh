#!/bin/sh
#
# Brings the zosmf-mcp container up. Installed as a LaunchAgent by
# scripts/install-launch-agent.sh, and safe to run by hand.
#
# launchd starts this with a minimal environment: no login shell, no PATH to
# Homebrew or to Apple's container CLI, and the container apiserver is not
# running yet. All three are handled here.

set -eu

PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
export PATH

cd "$(dirname "$0")/.."

if ! command -v container >/dev/null 2>&1; then
	echo "Apple container CLI not found in $PATH" >&2
	exit 1
fi

# `container system start` is idempotent, but it is slow enough to be worth
# skipping when the apiserver is already up.
if ! container system status >/dev/null 2>&1; then
	echo "starting the container apiserver"
	container system start
fi

# An existing container is reused. Configuration or image changes need
# `make container-down && make container-up-direct`, not a restart.
if container start zosmf-mcp >/dev/null 2>&1; then
	echo "started the existing zosmf-mcp container"
	exit 0
fi

echo "no zosmf-mcp container yet — creating one"
exec make container-up-direct
