#!/bin/sh
# Claude PermissionRequest hook → mure status=blocked (awaiting user consent).
command -v mure >/dev/null 2>&1 || exit 0
exec mure emit status blocked
