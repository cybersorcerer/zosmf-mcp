#!/usr/bin/env bash
#
# Fails if tracked files contain installation-specific identifiers: private
# IP addresses, internal hostnames, login@host pairs, or private keys.
#
# The check is pattern-based on purpose. A denylist of the actual values
# would have to name them, which is the leak it is meant to prevent.
#
# Documentation examples must use the ranges reserved for that purpose:
# example.com/.org/.net (RFC 2606) and 192.0.2.0/24, 198.51.100.0/24,
# 203.0.113.0/24 (RFC 5737).

set -uo pipefail

cd "$(dirname "$0")/.."

# Binaries and this script itself (which necessarily spells out the patterns)
# are not source material.
files=$(git ls-files | grep -vE '^(scripts/check-identifiers\.sh|LICENSE)$')

status=0

report() {
	local label=$1 matches=$2
	if [ -n "$matches" ]; then
		echo "$label:"
		echo "$matches" | sed 's/^/  /'
		echo
		status=1
	fi
}

# RFC 1918 ranges. 0.0.0.0 and 127.0.0.1 are not location-revealing and are
# left alone.
private_ip='(^|[^0-9.])(10|192\.168|172\.(1[6-9]|2[0-9]|3[01]))\.[0-9]{1,3}\.[0-9]{1,3}([^0-9.]|$)'
report "Private IP address" \
	"$(echo "$files" | xargs grep -InE "$private_ip" 2>/dev/null)"

# Hostnames in TLDs that only resolve inside someone's network.
internal_host='[a-z0-9-]{2,}\.(lan|lab|intern|internal|corp|home|fritz\.box)\b'
report "Internal hostname" \
	"$(echo "$files" | xargs grep -InE "$internal_host" 2>/dev/null)"

# login@host, unless the host is a reserved documentation domain.
user_at_host='[a-zA-Z0-9._-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}'
report "login@host" \
	"$(echo "$files" | xargs grep -InE "$user_at_host" 2>/dev/null |
		grep -vE '@([a-zA-Z0-9.-]+\.)?example\.(com|org|net)\b')"

# A private key must never be committed, whatever it belongs to.
report "Private key" \
	"$(echo "$files" | xargs grep -Iln -- '-----BEGIN .*PRIVATE KEY' 2>/dev/null)"

if [ "$status" -eq 0 ]; then
	echo "no installation-specific identifiers found"
fi

exit "$status"
