#!/usr/bin/env bash
# Check the remake's sound rendering against laanwj's SunDog port.
#
# The remake renders the original's Dosound lists through a Go port of emu2149;
# laanwj's port renders the same lists through emu2149 itself. Both should
# agree sample for sample. This builds the reference tool, renders every sound
# both ways, and reports where they differ.
#
#   tools/compare_dosound_oracle.sh <disk.st> <path-to-laanwj-sundog-checkout>
set -euo pipefail

DISK=${1:?usage: compare_dosound_oracle.sh <disk.st> <laanwj-sundog-checkout>}
REFERENCE=${2:?usage: compare_dosound_oracle.sh <disk.st> <laanwj-sundog-checkout>}
BUILD_IMAGE=${BUILD_IMAGE:-sundog-remake-build:20260812}
WORK=${WORK:-$(mktemp -d)}
ROOT=$(cd "$(dirname "$0")/.." && pwd)

DOCKER_RUN=(docker run --rm --log-opt max-size=10m --log-opt max-file=3
    -u "$(id -u):$(id -g)" -e HOME=/tmp)

# The reference asks emu2149 for the YM2149 volume table, but that constant is
# zero and the selector reads zero as "no preference", so the build as written
# renders through the AY-3-8910 table instead. The ST has a YM2149, so the copy
# built here asks for the table by its index and both sides model one chip.
echo "building the reference tool"
mkdir -p "$WORK/reference"
cp "$REFERENCE"/tools/dosound/*.c "$REFERENCE"/tools/dosound/*.h \
   "$REFERENCE"/tools/dosound/Makefile "$WORK/reference/"
sed -i 's/PSG_setVolumeMode(psg, EMU2149_VOL_YM2149)/PSG_setVolumeMode(psg, 1)/' \
    "$WORK/reference/dosound.c"
grep -q 'PSG_setVolumeMode(psg, 1)' "$WORK/reference/dosound.c" ||
    { echo "the reference no longer selects its volume table the same way" >&2; exit 1; }
"${DOCKER_RUN[@]}" -v "$WORK/reference":/src -w /src \
    --entrypoint sh "$BUILD_IMAGE" -c 'make -s dosound'

echo "rendering both ways into $WORK"
mkdir -p "$WORK/ours" "$WORK/theirs"
"${DOCKER_RUN[@]}" -v "$ROOT":/work -v "$(dirname "$DISK")":/disk \
    -v "$WORK/reference":/ref -v "$WORK":/out \
    -v "${GOCACHE_HOST:-$HOME/.cache/sundog-gocache}":/cache -w /work \
    --entrypoint bash "$BUILD_IMAGE" -c '
set -e
export PATH=/usr/local/go/bin:$PATH GOPATH=/cache/gopath GOCACHE=/cache/build \
       GOMODCACHE=/cache/mod GOFLAGS=-buildvcs=false
# The reference passes emu2149 2 MHz, which this model reads as half the speed
# the machine runs at. Rendering at the same figure is what makes the two
# outputs comparable; the remake itself renders at the ST clock.
go run ./cmd/sundog-sound-export -disk "/disk/'"$(basename "$DISK")"'" \
    -out /out/ours -format wav -clock 2000000 >/dev/null
go run ./cmd/dosound-lists -disk "/disk/'"$(basename "$DISK")"'" > /out/lists.txt
while read -r name hex; do
    /ref/dosound "/out/theirs/$name.wav" "$hex" 15
done < /out/lists.txt
'

echo "comparing"
python3 "$ROOT/tools/compare_wav.py" "$WORK/ours" "$WORK/theirs"
