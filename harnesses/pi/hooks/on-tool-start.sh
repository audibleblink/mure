#!/bin/sh
# pi tool_execution_start → mure status=working with tool name.
exec mure emit status working --tool "${TOOL:-${PI_TOOL_NAME:-unknown}}"
