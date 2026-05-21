#!/bin/sh
# Claude Stop hook → final result (assistant message on stdin) then idle.
# Read stdin once; ship to `mure emit result`, then flip status to idle so
# the agent is shown as awaiting user input rather than still "working".
command -v mure >/dev/null 2>&1 || exit 0
payload=$(cat)
printf '%s' "$payload" | mure emit result -
exec mure emit status idle
