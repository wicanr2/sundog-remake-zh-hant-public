#!/usr/bin/env bash
set -euo pipefail

# 編譯並執行 p-code 對拍 oracle。直譯器來自 laanwj/sundog（MIT），不進本 repository。
#
# 用法：tools/psys_oracle.sh <laanwj checkout> <pcode-hex> [oracle 參數...]

src=${1:?用法：psys_oracle.sh <laanwj checkout> <pcode-hex> [參數...]}
shift
src=$(cd "$src" && pwd)
here=$(cd "$(dirname "$0")" && pwd)
out=${SUNDOG_ORACLE_BIN:-$HOME/cht/tmp/psys-oracle}
mkdir -p "$out"

if [ ! -x "$out/psys_oracle" ] || [ "$here/psys_oracle.c" -nt "$out/psys_oracle" ]; then
	docker run --rm --network none -u "$(id -u):$(id -g)" \
		--memory 1g --cpus 2 --pids-limit 128 \
		--log-opt max-size=10m --log-opt max-file=3 \
		-v "$src/src":/src:ro -v "$here":/tools:ro -v "$out":/out -w /out \
		gcc:13-bookworm \
		gcc -O1 -std=gnu11 -I/src -I/src/psys -o /out/psys_oracle \
		/tools/psys_oracle.c /src/psys/psys_interpreter.c /src/psys/psys_opcodes.c \
		/src/psys/psys_debug.c /src/psys/psys_set.c /src/psys/psys_task.c \
		/src/psys/psys_registers.c \
		>&2
fi

docker run --rm --network none -u "$(id -u):$(id -g)" \
	--memory 512m --cpus 1 --pids-limit 64 \
	--log-opt max-size=10m --log-opt max-file=3 \
	-v "$out":/out:ro --entrypoint /out/psys_oracle gcc:13-bookworm "$@"
