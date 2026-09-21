#!/bin/sh
#
# Copies the setup instructions into an agent's own directory. Nothing is
# installed anywhere without this being run on purpose.
#
#   scripts/install-skill.sh [claude|opencode|pi] [project|user]

set -eu

AGENT=${1:-claude}
SCOPE=${2:-project}
REPO=$(cd "$(dirname "$0")/.." && pwd)

case "$AGENT" in
claude)
	WRAPPER="$REPO/agents/claude/SKILL.md"
	case "$SCOPE" in
	project) TARGET="$REPO/.claude/skills/zosmf-mcp-setup" ;;
	user)    TARGET="$HOME/.claude/skills/zosmf-mcp-setup" ;;
	*) echo "unknown scope $SCOPE — use project or user" >&2; exit 2 ;;
	esac
	;;
opencode)
	WRAPPER="$REPO/agents/opencode/SKILL.md"
	case "$SCOPE" in
	project) TARGET="$REPO/.opencode/skills/zosmf-mcp-setup" ;;
	user)    TARGET="$HOME/.config/opencode/skills/zosmf-mcp-setup" ;;
	*) echo "unknown scope $SCOPE — use project or user" >&2; exit 2 ;;
	esac
	;;
pi)
	WRAPPER="$REPO/agents/pi/SKILL.md"
	case "$SCOPE" in
	project) TARGET="$REPO/.pi/skills/zosmf-mcp-setup" ;;
	user)    TARGET="$HOME/.pi/agent/skills/zosmf-mcp-setup" ;;
	*) echo "unknown scope $SCOPE — use project or user" >&2; exit 2 ;;
	esac
	;;
*)
	echo "unknown agent $AGENT — known: claude, opencode, pi" >&2
	exit 2
	;;
esac

# The wrapper and setup.md are copied together on purpose: the wrapper
# refers to the file next to it, and a copy of only one of the two leaves
# an agent pointing at nothing.
mkdir -p "$TARGET"
cp "$WRAPPER" "$TARGET/"
cp "$REPO/agents/setup.md" "$TARGET/"

echo "installed the zosmf-mcp setup instructions for $AGENT ($SCOPE):"
echo "  $TARGET"
