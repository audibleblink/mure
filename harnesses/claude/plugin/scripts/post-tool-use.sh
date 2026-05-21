#!/bin/sh
# Claude PostToolUse hook → mure status=working (tool finished, back to inference).
command -v mure >/dev/null 2>&1 || exit 0
exec mure emit status working
