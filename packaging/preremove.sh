#!/bin/sh
# Runs before the shortorder package is removed.
set -e

if command -v systemctl >/dev/null 2>&1; then
	systemctl stop shortorder.service || true
	systemctl disable shortorder.service || true
fi
