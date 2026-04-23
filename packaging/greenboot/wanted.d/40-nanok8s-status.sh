#!/bin/bash
# greenboot wanted check: advisory surface of the most recent nanok8s
# lifecycle event. Non-failing by design — the message shows up in
# MOTD and `journalctl -u greenboot-healthcheck.service`.
set -u

EVENT_FILE=/var/lib/nanok8s/state/last-event

if [ -s "$EVENT_FILE" ] ; then
    echo "nanok8s: $(cat "$EVENT_FILE")"
else
    echo "nanok8s: no events recorded"
fi
