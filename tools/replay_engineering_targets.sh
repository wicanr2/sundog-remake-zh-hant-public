#!/usr/bin/env bash
set -euo pipefail

# 在核准的 Docker 工具鏈內執行；只使用正常玩家輸入，不使用 debug flag、座標注入或發道具。
# 驗的是工程艙的分流器記帳：有東西可修才收一個分流器，沒有東西可修就不收。
weapon_save_path="${1:-/tmp/sundog-engineering-weapon-save.json}"
shield_save_path="${2:-/tmp/sundog-engineering-shield-save.json}"
system_save_path="${3:-/tmp/sundog-engineering-system-save.json}"
weapon_output="${weapon_save_path}.output.txt"
shield_output="${shield_save_path}.output.txt"
system_output="${system_save_path}.output.txt"
go_bin="${SUNDOG_GO_BIN:-go}"

# 開局→戰術開火（開火把武器充能清空）→工程艙→修理武器→保存→離開。
# 一次修理補 25 點充能，並收走五個分流器裡的一個。
printf '%s\n' p x fire b e w b s q \
  | "$go_bin" run ./cmd/sundog -save "$weapon_save_path" >"$weapon_output"
grep -q '武器充能: 0 | 護盾等級: 100/100' "$weapon_output"
grep -q '武器充能: 25 | 護盾等級: 100/100 | 船體狀態: 16/16 | 分流器: 4' "$weapon_output"
grep -q '武器充能恢復 25 點' "$weapon_output"

# 開局→工程艙→修理護盾→保存→離開。護盾本來就是滿的，這一步不該收分流器。
printf '%s\n' p e s b s q \
  | "$go_bin" run ./cmd/sundog -save "$shield_save_path" >"$shield_output"
grep -q '護盾等級: 100/100 | 船體狀態: 16/16 | 分流器: 5' "$shield_output"
! grep -q '護盾等級恢復 25 點' "$shield_output"

# 開局→工程艙→修理選定系統→保存→離開。分流器修的是「格子」，控制節點壞掉時
# 裝不上去，所以這一段驗的是「修不動就不收分流器」。
# D6 之後新遊戲照原版帶隨機損壞，躍遷驅動不再保證是完好的 16/16（實測會出現
# 15/16 且訊息是「壞掉的是控制節點，分流器裝不上去」）。**要驗的記帳規則沒變**，
# 所以不再把格數釘死，改驗規則本身：分流器不動、也沒有「恢復 25 點」。
printf '%s\n' p e e b s q \
  | "$go_bin" run ./cmd/sundog -save "$system_save_path" >"$system_output"
grep -q '修理選定系統: 躍遷驅動' "$system_output"
grep -q '護盾等級: 100/100 | 船體狀態: 16/16 | 分流器: 5' "$system_output"
! grep -q '選定船艦系統恢復 25 點' "$system_output"

printf '工程艙分流器記帳（修得動收一個、修不動不收）通過。\n'
