#!/bin/sh
# Runs on install and on upgrade, on both deb and rpm.
set -e

systemctl daemon-reload || exit 0
systemctl enable netlama-agent.service >/dev/null 2>&1 || true

if systemctl is-active --quiet netlama-agent.service; then
	# Upgrade: pick up the new binary.
	systemctl restart netlama-agent.service || true
elif grep -qs '^NETLAMA_TOKEN=.\+' /etc/netlama/agent.env; then
	systemctl start netlama-agent.service || true
else
	echo "netlama-agent: create the agent in the UI, put its token into"
	echo "  /etc/netlama/agent.env, then: systemctl start netlama-agent"
fi
