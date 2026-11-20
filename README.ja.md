# mbyte — Magic Byte Insert/Replace CLI

[English](README.md) | **日本語**

`mbyte` は、ファイル先頭のマジックバイト（ファイルシグネチャ）を **挿入（insert）** または
**置換（replace）** する小さな **Go 製** CLI です。ファイルアップロードの種別検証がどう行われ、
マジックバイト偽装でどう回避されうるかを、**CTF や許可された環境でのセキュリティ検証**を通じて
学ぶための教育用ツールです。

やることは単純（先頭にバイト列を足す／先頭 N バイトを上書き）ですが、それを **安全に** 行えます
— 名前付きプリセット、入力検証、SHA-256 によるペイロード確認、そして「署名が合っているだけ」と
「実際にその形式としてパースできる」の差を示す構造検証（`check`）を備えています。

> 元々 Python 実装で、現在は Go へ移植済みです。Python 版は参照用として
> [`legacy-python/`](legacy-python/) に残しています。

---

## insert と replace の違い

| サブコマンド | 動作 | ファイルサイズ | 用途 |
| ------------ | ---- | -------------- | ---- |
| `insert`  | 先頭にバイト列を追加（元の中身は全部残る） | 増える | 署名さえ合格すれば中身は不問、という検証の回避 |
| `replace` | 先頭 N バイトを上書き（残りは保持） | 変わらない | 先頭数バイトだけを見る検証の回避 |

`insert` は元データを壊しません。`replace` は元ファイルが新しいバイト列より短いとエラーになります
（`insert` を使ってください）。

---

## インストール

### リリースバイナリ（推奨 / Go 不要）

[GitHub Releases](https://github.com/Akira0412-degitech/Mbyte/releases) から取得します。アーカイブ名は
`mbyte_<version>_<os>_<arch>.tar.gz`（Linux / macOS）または `.zip`（Windows）で、`checksums.txt`
（SHA-256）も同梱。Linux / macOS / Windows × amd64 / arm64 を用意しています。

Linux（amd64）の例:

```bash
VER=0.2.0
BASE="https://github.com/Akira0412-degitech/Mbyte/releases/download/v${VER}"
curl -fsSLO "${BASE}/mbyte_${VER}_linux_amd64.tar.gz"
curl -fsSLO "${BASE}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt      # 検証
tar xzf "mbyte_${VER}_linux_amd64.tar.gz"
sudo install -m 0755 mbyte /usr/local/bin/mbyte
```

Windows は `.zip` を展開し、`mbyte.exe` を PATH に置いてください（検証は `Get-FileHash ... -Algorithm
SHA256` で `checksums.txt` と突き合わせ）。インストール後は `mbyte update` で最新に保てます
（[自己更新](#自己更新update)参照）。

### go install / ソースからビルド

```bash
go install github.com/Akira0412-degitech/Mbyte/cmd/mbyte@latest   # Go 1.21 以上
# または
git clone https://github.com/Akira0412-degitech/Mbyte.git && cd Mbyte/go && go build -o mbyte ./cmd/mbyte
```

依存ライブラリ: [cobra](https://github.com/spf13/cobra)（CLI）と
[minio/selfupdate](https://github.com/minio/selfupdate)（`mbyte update`）。

---

## 使い方

```
mbyte insert  --preset <name>|--hex <hex> <input> -o <output>
mbyte replace --preset <name>|--hex <hex> <input> -o <output>
mbyte check   <file>
mbyte update  [--check]
mbyte --list-presets
```

### オプション（`insert` / `replace`）

| オプション | 説明 |
| ---------- | ---- |
| `--preset <name>` | 名前付きプリセットを使う（`--list-presets` で一覧） |
| `--hex <hex>` | 生の 16 進文字列。例: `89504e470d0a1a0a` |
| `-o`, `--output` | 出力パス（既定は `<input>.mbyte`、プリセット指定時はその拡張子） |
| `--force` | 入力ファイル自体への上書きを許可（既定は禁止） |
| `--raw` | （`replace` のみ）境界検出をせず、先頭から正確に `len(hex)` バイトを上書き |
| `-y`, `--yes` | 書き込み前の確認プロンプトをスキップ |
| `--dry-run` | 書き込まずプレビューのみ表示して終了 |
| `--no-check` | 書き込み後の構造チェックを行わない |
| `-r`, `--recursive` | 入力がディレクトリのとき再帰的に処理（バッチ） |
| `--json` | バッチ結果を JSON で出力 |
| `-v`, `--verbose` | 詳細ログ |

`--preset` と `--hex` はどちらか一方が必須。確認プロンプトは対話端末のときだけ表示され、パイプ・
スクリプト・バッチ実行では `-y` 相当でそのまま進みます。

### 実行例

リポジトリ同梱のサンプル `shell.php`（Web シェル文字列を含む無害なファイル）を使い、リポジトリ
ルートで実行します:

```bash
mbyte insert --preset png shell.php -o demo.png        # PNG ヘッダを先頭に追加
mbyte insert --hex 89504e470d0a1a0a shell.php -o x.png # 任意のシグネチャ
mbyte replace --preset jpeg demo.png -o demo.jpg       # 先頭署名を置換
mbyte replace --preset jpeg demo.png -o demo.jpg --dry-run   # プレビューのみ、書き込みなし
mbyte insert --preset png ./dir -o ./out -r --json     # ディレクトリを一括処理
```

成功した `insert` は、判定フォーマットと hexdump を標準出力に、構造チェック結果を標準エラーに
表示します:

```
[+] wrote demo.png
[+] detected format: PNG image data
[check] structure: FAIL (png: invalid format: Bad chunk length: 4294851584)
```

この `FAIL` は意図どおりです。PNG 署名を付けただけでは正規の PNG にはなりません（下記参照）。

### `replace` と `--raw`

既定の `replace` は、ファイルの既存署名が終わる位置を検出してそこで差し替えます。新しい署名が
元より短くても長くても正しく動き、ゴミバイトの残留やデータ破壊を避けられます。現在の署名を認識
できない場合はエラーになります。`--raw` で固定長上書きを強制するか、単に先頭へ足すなら `insert`
を使ってください。

---

## 構造検証（`check`）

`check` は、署名の判定と、Go 標準ライブラリのデコーダによる **実際のパース** の両方を 1 レポートに
まとめ、「署名が一致する」ことと「本当に妥当」なことの差を示します。

```bash
mbyte check shell.png.php
```

```
file: shell.png.php
[signature] PNG image data (matched 8 bytes)
[structure] FAIL (png: invalid format: Bad chunk length: 4294851584)
```

- 対応: PNG / JPEG / GIF、ZIP、ELF / PE。妥当なら `PASS`、未対応は `SKIP`、FAIL は終了コード `1`。
- `insert` / `replace` は書き込み後に自動で同じチェックを実行します（`--no-check` で無効化）。
- あわせてペイロードの SHA-256 を前後で比較し、`[MATCH]` / `[MISMATCH（±N バイトずれ）]` /
  `[UNVERIFIED]` を表示します（`insert` と境界検出ありの `replace` は常に `[MATCH]`）。

ZIP はファイル末尾の End of Central Directory を見るため、先頭に署名を足しただけでは妥当になりません
— 先頭バイトだけの偽装が本物のパーサを通せないことの好例です。

---

## 自己更新（`update`）

```bash
mbyte update --check    # 現在と最新を表示するだけ（何も変更しない）
mbyte update            # 最新をダウンロードして自己置換（TTY では確認）
mbyte update -y         # 確認なしで自己置換
```

`update` は、お使いの OS / アーキ向けの最新リリースを取得し、**アーカイブを `checksums.txt` と
照合 → 展開後バイナリの SHA-256 を再検証してから** 実行中のバイナリを置き換えます。ダウンロードや
検証のいずれかが失敗した場合、既存のバイナリには一切手を付けません。

| オプション | 説明 |
| ---------- | ---- |
| `--check` | 現在と最新を確認・表示するだけ。ダウンロードも置換もしない |
| `-y`, `--yes` | `[y/N]` 確認プロンプトをスキップ |

---

## プリセット

| 名前 | Hex | 名前 | Hex |
| ---- | --- | ---- | --- |
| `png` | `89504e470d0a1a0a` | `elf` | `7f454c46` |
| `gif89a` | `474946383961` | `pe` | `4d5a` |
| `gif87a` | `474946383761` | `gzip` | `1f8b` |
| `jpeg` | `ffd8ffe0` | `7z` | `377abcaf271c` |
| `pdf` | `25504446` | `rar` | `52617221` |
| `zip` | `504b0304` | `javaclass` | `cafebabe` |

CLI からは `mbyte --list-presets` で確認できます。（BMP は検出専用で、insert プリセットとしては未公開。）

---

## 終了コード

| コード | 意味 |
| ------ | ---- |
| `0` | 正常終了 |
| `1` | 実行時エラー（不正 hex・ファイル不存在・short replace・上書き拒否・未知プリセット・バッチ失敗・`check` の構造 FAIL 等）。`error: ...` を標準エラーに表示 |
| `2` | 引数エラー（例: `--preset` と `--hex` の同時指定、不明なフラグ） |

---

## 注意点・既知の制約

- **判定は「先頭バイトの推測」** で完全な検証ではありません。本物の `file(1)` と食い違うこともあります。
  実際にパースできるかは `mbyte check` で確認できます（それでも完全ではありません）。
- **`insert` は正規フォーマットとしては壊れたファイルを作り**（オフセットがずれる）、
  **`replace` は元の先頭バイトを失います**（入力ファイル自体は変更しません）。
- **万能な回避ツールではありません**。Content-Type チェック、再エンコード、実際のパースを伴う検証は
  マジックバイト偽装だけでは通りません。
- **`replace` の境界検出は上記プリセットのみ対応**。他の形式は `--raw` が必要です。
  **JPEG 検出は汎用の 3 バイト `FF D8 FF`** に一致します（プリセットは 4 バイトを書き込みます）。
- **`checksums.txt` は GPG 署名されていません** — SHA-256 は破損を防ぎますが、リリースホストの侵害や、
  両ファイルを偽造する中間者攻撃は防げません。高い保証が必要なら信頼できる経路で検証してください。
- **ウイルス対策ソフト** が Web シェル文字列を含むファイルを隔離することがあります。検証用フォルダを
  除外するか、無害なデータで試してください。

---

## 開発

```bash
cd go && go test ./... -v                 # Go（主）
cd legacy-python && pip install -e ".[test]" && pytest -q   # Python 参照実装
```

CI（GitHub Actions）で両方をビルド・テストします。シェル補完は
`mbyte completion <bash|zsh|fish|powershell>` で生成できます。

---

## 免責事項

**CTF の練習** と **明示的な許可を得た環境** でのセキュリティ検証のみを目的としています。所有も検証
許可もないシステムに対してアップロード制御を回避する目的での使用は違法となる可能性があります。作者は
悪用を是認せず、悪用による結果について責任を負いません。責任を持って利用してください。

## ライセンス

MIT — 詳細は [LICENSE](LICENSE) を参照。
