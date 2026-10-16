# 文件索引

- [專案介紹、故事與 HD 畫廊](../README.md)
- [五語 HD 正常玩家截圖](../README.md#五語-hd-遊玩截圖)
- [目前狀態與驗證限制](../CONTEXT.md)
- [建置及測試](build.md)
- [五語資料與韓文 TSV 來源](i18n/README.md)
- [精訊故事影片與重建工具](promo/README.md)
- [v.1.2.0 五語與三平台驗收](atari-st/release-v1200-20261016.json)
- [完整繁中操作說明](manual/remake-controls-zh-Hant.md)
- [倉庫 Swap 正常玩家驗證工具](../tools/verify_warehouse_swap_player.py)
- [連續航行燃料父層原生答案與驗證範圍](atari-st/fuel-flight-parent-parity-20261016.json)
- [schema44正常購油、兩補油入口與五語畫面驗收](atari-st/fuel-normal-player-parity-20261016.json)
- [正常燃料玩家驗證工具](../tools/verify_fuel_player.py)
- [schema45 戰鬥持久狀態](../internal/game/enemy_course.go)
- [schema45 正常新局、購貨與戰鬥保存工具](../tools/verify_schema45_player.py)
- [公開檔案與截圖雜湊](../PUBLIC-SOURCE-MANIFEST.json)
- [字型來源與授權](../assets/fonts/ASSET-MANIFEST.md)
- [工作紀錄](../WORKLOG.md)
- [工作規約](../AGENTS.md)

`docs/i18n/` 保存執行期翻譯資料；`docs/atari-st/` 僅保留普通單元測試讀取的獨立解碼事實及受控原版答案。原始磁碟、原版對拍圖片及完整研究歷程未隨公開 repo 提供。

`cmd/` 為程式入口，`internal/` 為規則、解析器及呈現，`tools/` 保存建置與研究工具，`docker/` 保存工具鏈。研究工具可能需要私人證據或自備磁碟，請先檢查參數與說明。

Swap 驗證使用自備合法磁碟、正常Dramming取貨保存及足以購買兩批貨物的資金；參數見工具 `--help`。`--review-only` 從已有正常倉庫保存重拍五語畫面，不執行購買或交換。這些輸入不隨公開來源提供。

schema45 玩家工具在 Docker 從標題建角，或用 `--library-input` 接續同一正常保存。地面往返來源交易所使用既有 Apple II 格移動，交易與戰術切回 Atari ST HD；不把這段測試稱為 Atari ST 像素地面對拍。原生連續戰鬥工具參數 `--combat-continuous-parent-only` 僅驗非隱形模式0至7、敵型0至6及明示受控亂數，新的批量數值答案留私人。

公開來源匯出腳本 [prepare_public_repository.py](../tools/prepare_public_repository.py) 供原私人工作樹建立新快照，使用 [public-source 範本](../tools/public-source/README.md)；公開 clone 已是匯出結果，不具備再次匯出所需的私人截圖路徑。

`verify_schema45_player.py --combat-reentry-only` 從正常有貨庫續行；`--shore-visits 4 --require-mode0` 預先固定下船／走回船門後的模式0抽驗，不反覆重抽。輸出使用專案 `workplace/` 內新目錄供Go重播。保存輸入留本機。

連續核心工具已覆蓋原始七型敵艦，含特殊UNKNOWN6的數值；每型144初態／1728更新，初始護盾255。缺型、未知索引或不相等拒絕通過。新原生數值收據留私人，不宣稱全型GUI、死亡或完整戰鬥。


低護盾0／1／7的七型非隱形連續父鏈，各1008初態／12096更新，合計3024／36288的22欄零差異；原版矩陣全部存活。另有schema45敵型2／模式0正常HD敗北保存重開，132欄與凍結時計通過。正式規則不變，原版死亡父層、隱形與全型GUI仍未知，新批量答案與玩家保存留私人。
研究參數與正常玩家入口見自寫tools/verify_talos_combat_cadence.py及tools/verify_combat_outcome_player.py。全cmd/internal、vet及51工具檢查通過。


船體歸零後立即中止剩餘穿透結算已修正。6720初態含624死亡，21物理欄與terminal投影零差異；修正前Counter552處不同，原生答案及6096存活正式案完全保持。手動／清醒／睡眠與正常schema45 HD敗北保存重開通過，全cmd/internal／vet及60工具檢查通過。完整原版死亡外層／GUI、隱形與全型GUI仍待，原生bulk和玩家保存留私人。

- [船體歸零當拍驗證工具](../tools/verify_talos_combat_death.py)
- [致命當拍差異護欄](../tools/test_verify_talos_combat_death.py)
- [正常HD勝敗與保存重開工具](../tools/verify_combat_outcome_player.py)
