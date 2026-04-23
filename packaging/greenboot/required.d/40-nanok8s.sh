#!/bin/bash
# greenboot required check: nanok8s.service must have completed
# successfully this boot. The service's own exit code already reflects
# "kubelet reached /readyz", so there is nothing to re-verify here;
# greenboot failing this script triggers bootc rollback.
set -eu

if ! systemctl is-active --quiet nanok8s.service ; then
    echo "nanok8s.service is not active" >&2
    systemctl status --no-pager nanok8s.service >&2 || true
    exit 1
fi
