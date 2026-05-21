#!/bin/sh
# Claude PreToolUse hook → mure status=working with tool name.
# Hook payload arrives on stdin as JSON; tool name is also in $CLAUDE_TOOL_NAME.
exec mure emit status working --tool "${CLAUDE_TOOL_NAME:-unknown}"
