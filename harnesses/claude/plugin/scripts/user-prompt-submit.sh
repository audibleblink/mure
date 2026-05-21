#!/bin/sh
# Claude UserPromptSubmit hook → mure status=working (inference begins).
command -v mure >/dev/null 2>&1 || exit 0
exec mure emit status working
