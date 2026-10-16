#!/bin/sh
# Capture the normal remake-owned ground screen in the Remake Modern look.
# Presentation is one axis of three presets since 2026-09-03, so this walks F8
# as many times as the wanted preset sits along the cycle: the default is the
# Atari ST look, one press is Apple II, two is Remake Modern. Run only inside
# the pinned Docker build image; the Xvfb process is bounded by this script and
# its trap.
set -eu

output_dir=${1:?usage: capture_modern_apple_ground.sh OUTPUT_DIR [F8_PRESSES] [OUTPUT_NAME]}
preset_presses=${2:-2}
output_name=${3:-modern-apple-ground-v1.png}
binary_path=/tmp/sundog-modern-apple-ground.bin
display_address=:98
runtime_dir=/tmp/sundog-modern-apple-runtime
mkdir -p "$output_dir" "$runtime_dir"
chmod 700 "$runtime_dir"

cleanup() {
	if [ -n "${app_pid:-}" ]; then
		kill "$app_pid" 2>/dev/null || true
		wait "$app_pid" 2>/dev/null || true
	fi
	if [ -n "${xvfb_pid:-}" ]; then
		kill "$xvfb_pid" 2>/dev/null || true
		wait "$xvfb_pid" 2>/dev/null || true
	fi
}
trap cleanup EXIT HUP INT TERM

go build -buildvcs=false -o "$binary_path" ./cmd/sundog-ebiten
Xvfb "$display_address" -screen 0 1280x800x24 -nolisten tcp >/tmp/sundog-modern-apple-ground-xvfb.log 2>&1 &
xvfb_pid=$!

DISPLAY="$display_address" XDG_RUNTIME_DIR="$runtime_dir" "$binary_path" -start ground >/tmp/sundog-modern-apple-ground.log 2>&1 &
app_pid=$!

window=""
attempt=0
while [ "$attempt" -lt 50 ]; do
	window=$(DISPLAY="$display_address" xdotool search --onlyvisible --name "陽犬號 SunDog remake" 2>/dev/null | head -n 1 || true)
	if [ -n "$window" ]; then
		break
	fi
	attempt=$((attempt + 1))
	sleep 0.1
done
if [ -z "$window" ]; then
	cat /tmp/sundog-modern-apple-ground.log >&2 || true
	echo "找不到 SunDog Ebiten 視窗" >&2
	exit 1
fi

# Start on the default preset (the Atari ST look). Each F8 event walks one step
# along the preset cycle; two of them land on Remake Modern, whose raster atlas
# is what this capture is for.
sleep 0.6
press=0
while [ "$press" -lt "$preset_presses" ]; do
	DISPLAY="$display_address" xdotool key --window "$window" --delay 120 F8
	sleep 0.7
	press=$((press + 1))
done
sleep 0.9

capture_path="$output_dir/$output_name"
DISPLAY="$display_address" import -window "$window" "$capture_path"
sha256sum "$capture_path"
