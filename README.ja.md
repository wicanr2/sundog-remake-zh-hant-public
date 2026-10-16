# SunDog：Atari ST HD リメイク

[繁體中文](README.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [English](README.en.md)

Go と Ebitengine で作り直した、宇宙交易と冒険のゲームです。Atari ST の画面構成を基準とし、独自に描いた HD グラフィックを追加しました。繁体字中国語、簡体字中国語、日本語、韓国語、英語に対応しています。

現在のバージョンは **v.1.2.0-20261016** です。プレイには、合法的に入手した Atari ST 版の `sundog.st` が必要です。公開ソースとエンジン配布物に、原作ディスク、画像、音声、説明書のスキャンは含まれません。元の非公開リポジトリの履歴、比較画像、旧 Release は非公開のまま保存しています。

![通常のキャラクター作成後に入る HD 船内](docs/screenshots/hd-shipboard.png)

## 物語

舞台は Jebal 帝国保護領の Drahew 地域。12 の星系と 18 の有人惑星の間を、商船が生活物資や開拓資材を運んでいます。あなたは貨物船 SunDog を操り、宇宙港で交易しながら未知の世界を訪れます。

主人公 Zed はガラス鉱山で働いていました。疎遠だった叔父 Brock Dor-Ceed の死により、傷だらけの宇宙貨物船と、各地の銀行口座のコードを相続します。それは鉱山を離れ、自分の人生を選ぶ初めての機会でした。

しかし遺産には、叔父の借金と未完了の開拓契約も含まれていました。Jondd の Banville 開拓地を探し、建設物資を届け、各地の倉庫に保管された冷凍移民を運ばなければなりません。交易で燃料代と修理代を稼ぎ、次の航海に備えましょう。海賊は積荷を、街の強盗は現金を狙います。船を維持して開拓地を育てることが、Zed の自由を守る道です。

この紹介は Atari ST 版『SunDog: Frozen Legacy』説明書の 3–4 ページを要約したものです。説明書やスキャンは配布していません。Zed は説明書の主人公名で、ゲーム内では自分で名前を付けられます。

## Atari ST HD

明るい色彩、船の輪郭、画面の区分、操作の意味を維持しながら、タイトル、船内、航法画面、都市、惑星地表、室内、人物、物品、機関室、戦術、各施設とエンディングを描き直しました。独立した素材には 330 のタイル、17 種類の室内背景、31 種類の物品アイコンと空欄が含まれます。

`F8` で **Atari ST → Atari ST HD → Apple II** と切り替えます。初期設定は Atari ST です。選択は保存され、`F2` からも変更できます。原作表示に必要な画像は、プレイヤーのディスクから読み込みます。

| 都市と停泊船 | 星系航法 |
|---|---|
| ![HD 都市](docs/screenshots/hd-ground.png) | ![HD 星系図](docs/screenshots/hd-system.png) |
| 機関室 | 船内物資 |
| ![HD 機関室](docs/screenshots/hd-engineering.png) | ![HD 物資](docs/screenshots/hd-stores.png) |

画像は正式統合候補を通常操作した際の記録です。それぞれ異なるプレイ分岐から取得しており、同一状態の原作比較ではありません。ハッシュは[公開ソース一覧](PUBLIC-SOURCE-MANIFEST.json)に記録しています。

## 5言語の HD ゲーム画面

同じセーブデータの Dramming 倉庫で、ゲーム内の設定から言語を切り替えて撮影しました。画像をクリックすると 1280×800 の原寸で表示できます。

<table>
<tr>
<td><a href="docs/screenshots/hd-warehouse-zh-Hant-20261016.png"><img src="docs/screenshots/hd-warehouse-zh-Hant-20261016.png" alt="繁體中文 · Atari ST HD · Dramming" width="416"></a><br>繁體中文</td>
<td><a href="docs/screenshots/hd-warehouse-zh-Hans-20261016.png"><img src="docs/screenshots/hd-warehouse-zh-Hans-20261016.png" alt="简体中文 · Atari ST HD · Dramming" width="416"></a><br>简体中文</td>
</tr>
<tr>
<td><a href="docs/screenshots/hd-warehouse-ja-20261016.png"><img src="docs/screenshots/hd-warehouse-ja-20261016.png" alt="日本語 · Atari ST HD · Dramming" width="416"></a><br>日本語</td>
<td><a href="docs/screenshots/hd-warehouse-ko-20261016.png"><img src="docs/screenshots/hd-warehouse-ko-20261016.png" alt="한국어 · Atari ST HD · Dramming" width="416"></a><br>한국어</td>
</tr>
<tr>
<td><a href="docs/screenshots/hd-warehouse-en-20261016.png"><img src="docs/screenshots/hd-warehouse-en-20261016.png" alt="English · Atari ST HD · Dramming" width="416"></a><br>English</td>
</tr>
</table>

現在のソースから通常の操作でセーブを読み込んだ、未加工の画面です。倉庫の「交換」は v.1.2.0-20261016 公開後に追加した機能で、このバージョンの配布パッケージには含まれません。画像のハッシュは[公開ソース一覧](PUBLIC-SOURCE-MANIFEST.json)に記録しています。

## ダウンロードと言語設定

[v.1.2.0-20261016 のエンジン](https://github.com/wicanr2/sundog-remake-zh-hant-public/releases/tag/v.1.2.0-20261016)をダウンロードするか、[Docker ビルド手順](docs/build.md)でコンパイルしてください。起動後に合法的に所有する `sundog.st` を選びます。自分のディスクから効果音を取り込むまでは無音です。

`F2` を開き、言語の行で `←`／`→` を押すとすぐに切り替わります。再起動後も設定は維持されます。`-locale ja`、`-locale ko`、`-locale en` でも起動できます。言語はゲームの進行やセーブデータを変更しません。[語系データの説明](docs/i18n/README.md)も参照してください。翻訳には AI を使用しており、母語話者による全体の校閲は未実施です。

| プラットフォーム | 検証範囲 |
|---|---|
| Linux x86_64 | 通常起動、表示切替、保存と再読込 |
| Windows x64 | Wine 上の起動スクリプトと保存・再読込。実機未検証 |
| macOS Intel／Apple Silicon | アーキテクチャと署名の静的検査。実機未検証 |

原作データ入りの完全版と音声デモ動画はローカル専用です。

## 操作

| 操作 | キー |
|---|---|
| ヘルプ／設定／ミュート | `F1`／`F2`／`F7` |
| 表示／キャラクター保存庫 | `F8`／`F5` |
| 戻る／保存して終了 | `Esc` または右クリック／`Q` または `F10` |
| 船内機能 | `N` 航法、`X` 戦術、`E` 機関室、`B` 物資、`C` 貨物、`H` 状態、`G` 下船 |
| 移動／決定 | 方向キーまたはマウス／`Enter` または `Space` |
| キャラクター保存庫 | `M` 作成、`N` 新規、`1`–`8` スロット、`S` 保存、`L` 読込 |

各画面の操作は `F1` で確認できます。名前入力中の文字キーは名前の入力に使います。終了は `F10` です。ソースの main は新規 schema45 セーブに対応し、標準ファイル名に v45 を含みます。公開済み v.1.2.0-20261016 は schema44 です。旧セーブは旧版用に保管し、変換は行いません。保存先は Linux `~/.config/sundog-remake/`、Windows `%APPDATA%\sundog-remake\`、macOS `~/Library/Application Support/sundog-remake/` です。

## 検証とライセンス

通常操作の抽出検証は、新規作成、船内機能、航法、地上移動、都市施設、室内、航行中の睡眠、採点、失敗エンディング、保存と再読込を含みます。複数の言語とウィンドウサイズでも描画を検査しています。ソースの main ではモード1〜7とクローク未使用のモード0の進路変更カウントダウン、射撃準備、更新間の状態を保持し、原作データの敵艦7種類について、乱数入力を制御した連続処理の照合を通過しました。低シールド時の貫通ダメージも照合済みです。schema45の通常操作によるHD敗北と再読み込みを1例確認しました。船体が0になった時点で残りの貫通ダメージ処理を中断するよう修正し、準備済みの致命攻撃を照合しました。原作の死亡処理全体と画面、全遭遇と勝敗分岐は未検証です。クローク使用中のモード0は従来の近似処理を維持し、戦闘全体の一致は未検証です。[戦闘処理](internal/game/enemy_course.go)を参照してください。原作 GUI の全動作、自然乱数列の一致、全戦闘分岐、Windows／macOS 実機、聴覚上の同期には未検証部分が残ります。ゲーム全体の画素単位・フレーム単位の一致を主張するものではありません。

[文書索引](docs/index.md)と[現在の状態](CONTEXT.md)が入口です。一部の研究ツールは非公開の証拠を参照し、別途合法的な入力データを必要とします。

自作コード、ツール、翻訳、独立した美術には [RRSAL-1.0](LICENSE) を適用します。フォントは[出典とライセンス](assets/fonts/ASSET-MANIFEST.md)に記載された各条件に従います。ライセンスの繁体字中国語本文を正本とします。

『SunDog: Frozen Legacy』および原作素材の権利は各権利者に帰属します。本作は独立した研究リメイクであり、原作開発元との提携関係はありません。本ライセンスは原作素材の再配布権を与えません。
