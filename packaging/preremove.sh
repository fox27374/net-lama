#!/bin/sh
# $1 is "remove" (deb) or "0" (rpm) on an actual removal, something else
# during an upgrade — where the service must keep running.
set -e

case "$1" in
remove | 0)
	systemctl stop netlama-agent.service >/dev/null 2>&1 || true
	systemctl disable netlama-agent.service >/dev/null 2>&1 || true
	;;
esac
