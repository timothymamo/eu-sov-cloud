#!/usr/bin/env bash
# Merges dev/claude-user-settings.json into the Claude Code user settings (~/.claude/settings.json) of the
# VM user, with @WORKSPACE@ replaced by the workspace directory the VM mounts. Claude Code reads `autoMode`
# only from user settings, never from a repository's .claude/, so this part of the agents' configuration
# cannot be checked in as settings. Run it through `make vm-claude-settings`.
#
# The template's keys replace the same keys in the user settings, and permissions.allow is merged, so
# keys the template does not name (theme, skipDangerousModePermissionPrompt, ...) are kept. Running it
# again is safe; a rule removed from the template has to be removed from the VM by hand.
set -euo pipefail

workspace="${1:?usage: $0 <workspace directory>}"
template="$(cd "$(dirname "$0")" && pwd)/claude-user-settings.json"
settings="${HOME}/.claude/settings.json"

# Provisioned by dev/lima.yaml, so this never rewrites the Mac's own settings by mistake.
if [ ! -f /etc/profile.d/shift2eu.sh ]; then
  echo "claude-settings: run this inside the Lima VM (make vm-claude-settings)" >&2
  exit 1
fi

mkdir -p "$(dirname "${settings}")"
[ -s "${settings}" ] || echo '{}' >"${settings}"

tmp="$(mktemp)"
jq --slurp --arg workspace "${workspace}" '
  .[0] as $user
  | (.[1] | walk(if type == "string" then gsub("@WORKSPACE@"; $workspace) else . end)) as $template
  | ($user * $template)
  | .permissions.allow = ((($user.permissions.allow // []) + ($template.permissions.allow // [])) | unique)
' "${settings}" "${template}" >"${tmp}"
mv "${tmp}" "${settings}"

echo "claude-settings: merged into ${settings}; check with 'claude auto-mode config'"
