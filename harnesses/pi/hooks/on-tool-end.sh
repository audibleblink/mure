#!/bin/sh
# pi tool_execution_end → mure status=idle (briefly; agent_end clears finally).
exec mure emit status idle
