#!/bin/sh
# Runs after the shortorder package is installed/upgraded (deb/rpm/apk).
set -e

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload || true
	systemctl enable shortorder.service || true
	# Start (or restart on upgrade) so it's live immediately and on every boot.
	systemctl restart shortorder.service || true
fi

echo "shortorder installed. Web UI + JSON API on http://<this-host>/"
echo "  status: sudo systemctl status shortorder"
echo "  logs:   journalctl -u shortorder -f"
echo "Plug in an Epson-compatible ESC/POS USB thermal printer (e.g. a Volcora v-WRP2-A1W) and it is detected automatically."

# Raspberry Pi appliance setup (best-effort, never fails the install).
if grep -qi "Raspberry Pi" /proc/device-tree/model 2>/dev/null; then
	echo ""
	echo "Raspberry Pi detected — configuring as a 'shortorder' appliance..."

	# Set the hostname to 'shortorder' so it is reachable at http://shortorder.local/
	if command -v hostnamectl >/dev/null 2>&1; then
		hostnamectl set-hostname shortorder 2>/dev/null || true
	else
		echo shortorder >/etc/hostname 2>/dev/null || true
	fi
	# Keep /etc/hosts in sync so sudo/mDNS resolve the new name without warnings.
	if [ -f /etc/hosts ] && ! grep -q "127.0.1.1[[:space:]]*shortorder" /etc/hosts; then
		printf '127.0.1.1\tshortorder\n' >>/etc/hosts || true
	fi

	echo "  Hostname set to 'shortorder'. After a reboot, reach it at http://shortorder.local/"
fi
