# SunDog: Atari ST HD Remake

[繁體中文](README.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [English](README.en.md)

A science fiction trading adventure rebuilt in Go and Ebitengine, with the Atari ST interface and independently drawn HD artwork. The game supports Traditional Chinese, Simplified Chinese, Japanese, Korean and English.

Current version: **v.1.2.0-20261016**. You need your own legally obtained Atari ST `sundog.st` disk. This repository and its engine releases do not include original disks, graphics, sounds or scanned manuals. The original private repository's history, comparison images and earlier releases remain private.

![HD cabin after normal character creation](docs/screenshots/hd-shipboard.png)

## The story

In the Drahew region of the Jebal Imperial Protectorate, merchant ships carry supplies between 12 star systems and 18 inhabited planets. You command the SunDog, trading in spaceports and travelling to unfamiliar worlds.

Zed has spent his life working in glass mines. When his estranged uncle Brock Dor-Ceed dies, he inherits a battered star freighter and codes for bank accounts scattered across the region. For the first time, he has a chance to leave the mines.

The inheritance also brings debts and an unfinished colonization contract. Find Banville on Jondd, deliver construction supplies and transport cryogenically frozen colonists from distant warehouses. Trading must pay for fuel, repairs and each new journey. Pirates may demand your cargo, and street robbers may demand your cash. Keep the ship running and the colony growing to protect Zed's new freedom.

This introduction paraphrases pages 3–4 of the Atari ST *SunDog: Frozen Legacy* manual. The manual and its scans are not included. Zed is the manual's protagonist; you can name your own character in the game.

## Atari ST HD

The HD presentation preserves the bright colours, ship silhouette, interface regions and action meanings of the Atari ST reference. It covers the title, cabin, navigation, cities, planetary surfaces, interiors, people, items, engineering, tactical views, services and endings. Independent artwork includes 330 tiles, 17 interior backgrounds and 31 item icons plus an empty slot.

Press `F8` to cycle **Atari ST → Atari ST HD → Apple II**. Atari ST remains the default. Your choice is remembered, and `F2` also offers presentation selection. Original presentations read their graphics from your own disk.

| City and parked ship | System navigation |
|---|---|
| ![HD city](docs/screenshots/hd-ground.png) | ![HD system map](docs/screenshots/hd-system.png) |
| Engineering | Shipboard stores |
| ![HD engineering](docs/screenshots/hd-engineering.png) | ![HD stores](docs/screenshots/hd-stores.png) |

These are normal player captures from the integrated HD candidate, taken on different branches. They are not original comparison images or a claim that all scenes share one state. File hashes are recorded in the [public source manifest](PUBLIC-SOURCE-MANIFEST.json).

## HD screenshots in five languages

The same saved warehouse in Dramming, captured after switching languages through the game interface. Click an image for the original 1280×800 capture.

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

These unedited normal-play captures show the current source branch. Warehouse Swap was added after v.1.2.0-20261016 and is not included in that release’s installers. Image hashes are recorded in the [public source manifest](PUBLIC-SOURCE-MANIFEST.json).

## Download and language

Download the [v.1.2.0-20261016 engine release](https://github.com/wicanr2/sundog-remake-zh-hant-public/releases/tag/v.1.2.0-20261016), or follow the [Docker build guide](docs/build.md). Select your legal `sundog.st` when prompted. Sound stays muted until you import sound from your own disk.

Open `F2`, then use `←` / `→` on the language row. Changes apply immediately and persist after restart. Command-line options include `-locale en`, `-locale ja` and `-locale ko`. Language does not change game state or save data. See the [language data guide](docs/i18n/README.md). Translations are AI-assisted and still await complete native-language editorial review.

| Platform | Validation scope |
|---|---|
| Linux x86_64 | Normal startup, presentation switching, save and reload |
| Windows x64 | Launcher and save/reload under Wine; native hardware untested |
| macOS Intel / Apple Silicon | Architecture and signature checks; native hardware untested |

Full packages containing original data and the sound demonstration video are local-only.

## Controls

| Action | Keys |
|---|---|
| Help / options / mute | `F1` / `F2` / `F7` |
| Presentation / character library | `F8` / `F5` |
| Back / save and quit | `Esc` or right-click / `Q` or `F10` |
| Cabin functions | `N` navigation, `X` tactical, `E` engineering, `B` stores, `C` cargo, `H` status, `G` disembark |
| Move / confirm | Arrow keys or mouse / `Enter` or `Space` |
| Character library | `M` create, `N` new game, `1`–`8` slot, `S` store, `L` load |

Use `F1` for each screen's actions. During naming, letter keys enter text; use `F10` to quit. Only schema44 saves are supported. Saves, the library and preferences use the system configuration folder: Linux `~/.config/sundog-remake/`, Windows `%APPDATA%\sundog-remake\`, macOS `~/Library/Application Support/sundog-remake/`.

## Verification and licence

Player sampling covers character creation, cabin functions, navigation, ground movement, city services, interiors, sleep during flight, scoring, failed endings and save/reload. Rendering checks cover multiple languages and window sizes. Complete original GUI behaviour, matching natural random streams, every combat branch, native Windows/macOS operation and perceived audio timing remain outside the verified scope. This project does not claim whole-game pixel or frame equivalence.

See the [document index](docs/index.md) and [current status](CONTEXT.md). Some research tools refer to evidence retained in the private project and need separately supplied legal inputs.

Our code, tools, translations and independent artwork use [RRSAL-1.0](LICENSE). Fonts retain their own licences, listed in the [font manifest](assets/fonts/ASSET-MANIFEST.md). The Traditional Chinese licence text is authoritative.

*SunDog: Frozen Legacy* and its original assets belong to their respective rights holders. This independent research remake is not affiliated with the original developers. Its licence does not grant redistribution rights to original assets.
