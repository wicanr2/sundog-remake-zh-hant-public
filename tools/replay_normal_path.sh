#!/usr/bin/env bash
set -euo pipefail

# Run this script from the repository root inside the approved Docker toolchain.
# It intentionally uses only normal player commands; there is no debug flag,
# coordinate injection, item grant, or forced-win action.
save_path="${1:-/tmp/sundog-normal-path-save.json}"
library_path="${2:-/tmp/sundog-normal-path-library.json}"
go_bin="${SUNDOG_GO_BIN:-go}"

printf '%s\n' \
  c m n 陽犬測試 confirm \
  b c s n 陽犬任務 confirm \
  g p k b n go g deliver b x off fire fire fire fire b h e b e e b s q \
  | "$go_bin" run ./cmd/sundog -save "$save_path" -library "$library_path"
