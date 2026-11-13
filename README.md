# mbyte — Magic Byte Insert/Replace CLI

`mbyte` は、ファイル先頭のマジックバイト（ファイルシグネチャ）を **挿入（insert）** または
**置換（replace）** するための、小さな **Go 製** CLI ツールです。

ファイルアップロード時のファイル種別検証がどのように行われ、どのように回避されうるか
（マジックバイト偽装）を **CTF や、許可された環境でのセキュリティ検証** を通じて学ぶための
教育用ツールです。

やっていること自体は単純で、「先頭にバイト列を足す」か「先頭の N バイトを上書きする」だけです。
価値は、その単純な操作を **安全に・わかりやすく** 行える点にあります — 名前付きプリセット、
入力バリデーション、処理後に `file` / `xxd` 相当の結果を表示する確認機能、ペイロードが無傷か
を確かめる SHA-256 比較、そして「署名が合っているだけ」と「実際にその形式としてパースできる」
の違いを示す構造検証（`check`）を備えています。

> このリポジトリは元々 Python で実装され、現在 Go へ移植済みです。元の Python 実装は参照用として
> [`legacy-python/`](legacy-python/) に保存しています（削除していません）。

---

## insert と replace の違い

この 2 つは似ていますが結果がまったく異なります。用途に合わせて選んでください。

| サブコマンド | 動作 | ファイルサイズ | 主な用途 |
| ------------ | ---- | -------------- | -------- |
| `insert`  | 先頭に指定バイト列を **追加**（元の中身は全部残る） | 増える | シグネチャさえ合格すれば中身は不問、という緩い検証の回避 |
| `replace` | 先頭 N バイトを指定バイト列で **上書き**（残りは保持） | 変わらない | 先頭数バイトだけを見て判定するような検証の回避 |

- `insert` は元データを壊さないので、画像ビューアでは壊れた画像扱いでも、中身は元のまま読めます。
- `replace` は元ファイルが指定バイト列より短いとエラーになります（`insert` を使ってください）。

---

## インストール

### `go install`（Go ツールチェインがある場合）

```bash
go install github.com/Akira0412-degitech/Mbyte/cmd/mbyte@latest
```

`mbyte` バイナリが `$(go env GOPATH)/bin` に入ります（PATH を通してください）。

### リリースバイナリをダウンロード

[GitHub Releases](https://github.com/Akira0412-degitech/Mbyte/releases) から各 OS/アーキテクチャ向けの
アーカイブ（Linux / macOS / Windows、amd64 / arm64）を取得し、`mbyte`（Windows は `mbyte.exe`）を
PATH の通ったディレクトリに置くだけで動きます。

### ソースからビルド

```bash
cd go
go build -o mbyte ./cmd/mbyte
```

- 必要環境: **Go 1.21 以上**
- 依存ライブラリ: CLI 実装に [cobra](https://github.com/spf13/cobra) を使用（`go.mod` 参照）。

---

## 使い方

```
mbyte insert  --preset <name> <input> -o <output>
mbyte insert  --hex <hexstring> <input> -o <output>
mbyte replace --preset <name> <input> -o <output>
mbyte replace --hex <hexstring> <input> -o <output>
mbyte check   <file>
mbyte --list-presets
```

### オプション（`insert` / `replace`）

| オプション | 説明 |
| ---------- | ---- |
| `--preset <name>` | 名前付きプリセットを使う（`--list-presets` で一覧） |
| `--hex <hexstring>` | 生の 16 進文字列を直接指定（例: `89504e470d0a1a0a`） |
| `-o`, `--output` | 出力ファイルパス。**省略時は `<input>.mbyte`**（プリセット指定時はそのプリセットの既定拡張子） |
| `--force` | 入力ファイル自体への上書きを許可（既定では事故防止のため禁止） |
| `--raw` | （`replace` のみ）署名境界検出をスキップし、先頭から正確に `len(hex)` バイトを上書き（旧来の危険な挙動） |
| `-y`, `--yes` | 書き込み前の確認プロンプトをスキップ |
| `--dry-run` | 書き込まず、プレビューだけ表示して終了 |
| `--no-check` | 書き込み後の構造チェック（デコード検証）を行わない |
| `-r`, `--recursive` | 入力がディレクトリのとき、サブディレクトリも対象にする（バッチ） |
| `--json` | 入力がディレクトリのとき、結果を JSON 配列で出力 |
| `-v`, `--verbose` | 詳細ログを表示 |
| `--list-presets` | プリセット一覧を表示して終了（ルート） |
| `--version` | バージョンを表示（ルート） |

- `--preset` と `--hex` は **どちらか一方が必須**（両方指定はエラー）。
- 出力先が入力ファイルと同一になる場合、`--force` がないと処理を拒否します（上書き事故防止）。
- **確認プロンプト**は端末（TTY）から対話実行したときだけ出ます。パイプやスクリプト等の非対話実行では
  そのまま実行されます（`-y` と同等）。ディレクトリへのバッチ処理も非対話で実行されます。

### 実行例

プリセットを使って PNG ヘッダを先頭に追加：

```bash
mbyte insert --preset png payload.bin -o payload.png
```

成功すると、判定フォーマットと先頭 16 バイトの hexdump が標準出力に、構造チェック結果が
標準エラーに表示されます：

```
[+] wrote payload.png
[+] detected format: PNG image data
00000000: 89 50 4e 47 0d 0a 1a 0a 70 61 79 6c 6f 61 64 00  .PNG....payload.
[check] structure: FAIL (unexpected EOF)
```

> 上の `[check] structure: FAIL` は意図どおりです。PNG シグネチャを付けただけでは正規の PNG には
> ならないことを示しています（下記「構造検証」参照）。

生の 16 進で任意のシグネチャを追加：

```bash
mbyte insert --hex 89504e470d0a1a0a notes.txt -o notes.png.txt
```

先頭バイトを JPEG シグネチャで置換：

```bash
mbyte replace --preset jpeg upload.dat -o upload.jpg
```

書き込み前に変更内容とペイロードハッシュを確認（書き込みなし）：

```bash
mbyte replace --preset jpeg upload.dat -o upload.jpg --dry-run
```

```
[preview] input    : upload.dat (12345 bytes)
[preview] current signature : 474946383961 ("GIF image data, version 89a", 6 bytes)
[preview] new signature     : ffd8ffe0 ("JPEG image data", 4 bytes)
[preview] payload hash (before)           : 9f86d0...
[preview] payload hash (after, estimated) : 9f86d0... [MATCH]
[preview] output   : upload.jpg (12343 bytes, -2 bytes)
[*] dry-run: no file written
```

ディレクトリ内の全ファイルへ一括適用（結果を JSON で）：

```bash
mbyte insert --preset png ./uploads -o ./out --recursive --json
```

### `replace`: 署名境界検出と `--raw`

既定の `replace` は、ファイルの **既存** 署名の長さを検出し、その境界で新しいバイト列を差し込みます。
新しい署名が元より短くても長くても正しく動きます（切り取り位置は「元の署名が実際に終わる場所」で
あって、新しい署名の長さではないため）。これにより、ゴミバイトの残留（新署名が短い場合）や実データの
破壊（新署名が長い場合）を避けられます。

```bash
# 安全（既定）：既存署名の実際の長さを検出してそこで差し替える
mbyte replace --preset png broken.png -o fixed.png

# 手動／危険：検出をスキップし先頭から len(hex) バイトを上書き
mbyte replace --preset png broken.png -o fixed.png --raw
```

現在の署名が認識できない場合、`replace`（`--raw` なし）は差し込み位置を推測せずエラーにします。
`--raw` で固定長上書きを強制するか、単に先頭へ足したいだけなら `insert` を使ってください。

---

## ペイロード整合性チェック

`insert` / `replace` は、操作前後で **ペイロード（署名以降のバイト列）** の SHA-256 を比較し、
`[MATCH]` / `[MISMATCH]` / `[UNVERIFIED]` を示します（`--dry-run` や `-v`、対話実行時のプレビューに表示）。

- `insert` と境界検出ありの `replace` はペイロードを保持するので常に `[MATCH]`。
- `--raw` で長さがズレると `[MISMATCH] (payload shifted by ±N bytes)` と、何バイトずれたかを表示します。

---

## 構造検証（`check`）

`detect_format`（先頭シグネチャの判定）と、Go 標準ライブラリのデコーダによる **実際のパース** を
1 レポートにまとめます。「署名が一致する」ことと「その形式として本当に妥当」なことの違いが分かります。

```bash
mbyte check shell.png
```

```
file: shell.png
[signature] PNG image data (matched 8 bytes)
[structure] FAIL (png: invalid format ...)
```

- 対応フォーマット: PNG / JPEG / GIF（`image/*`）、ZIP（`archive/zip`）、ELF / PE（`debug/elf` / `debug/pe`）。
- 構造が妥当なら `[structure] PASS`、未対応フォーマットは `[structure] SKIP`。
- 構造 FAIL のとき `check` は終了コード `1` を返します。
- `insert` / `replace` も書き込み後に自動で同じチェックを行い結果を標準エラーへ追記します（`--no-check` で無効化）。

ZIP は末尾の End of Central Directory を検証するため、先頭にシグネチャを足しただけでは決して妥当に
なりません。これは「先頭バイトだけの偽装」では本物の検証を通せないことの好例です。

---

## プリセット一覧

| 名前        | Hex                | フォーマット              |
| ----------- | ------------------ | ------------------------- |
| `png`       | `89504e470d0a1a0a` | PNG 画像                  |
| `gif89a`    | `474946383961`     | GIF 画像 (version 89a)    |
| `gif87a`    | `474946383761`     | GIF 画像 (version 87a)    |
| `jpeg`      | `ffd8ffe0`         | JPEG 画像                 |
| `pdf`       | `25504446`         | PDF ドキュメント          |
| `zip`       | `504b0304`         | ZIP アーカイブ            |
| `elf`       | `7f454c46`         | ELF 実行ファイル          |
| `pe`        | `4d5a`             | PE32 実行ファイル (MZ)    |
| `gzip`      | `1f8b`             | gzip 圧縮データ           |
| `7z`        | `377abcaf271c`     | 7-zip アーカイブ          |
| `rar`       | `52617221`         | RAR アーカイブ            |
| `javaclass` | `cafebabe`         | Java class ファイル       |

`mbyte --list-presets` で CLI からも確認できます。

---

## シェル補完

cobra の補完生成を利用できます（bash / zsh / fish / powershell）。例（bash）：

```bash
mbyte completion bash > /etc/bash_completion.d/mbyte      # システム全体
# or
mbyte completion bash > ~/.local/share/bash-completion/completions/mbyte
```

`mbyte completion --help` で各シェルの手順が見られます。

---

## 終了コードとエラー

| 状況 | 終了コード | 挙動 |
| ---- | ---------- | ---- |
| 正常終了 | `0` | 出力ファイルを書き込み、確認情報を表示 |
| 実行時エラー | `1` | 不正な 16 進・ファイル不存在・short replace・上書き拒否・未知プリセット／バッチ内の失敗／`check` の構造 FAIL など。`error: ...` を標準エラーに表示（バッチ・`check` は結果出力済みのため終了コードのみ） |
| 引数エラー | `2` | 引数自体の誤り（例: `--preset` と `--hex` の同時指定、不明なフラグ） |

主なエラーメッセージの例：

```
error: hex string has an odd number of digits (3): 'xyz'
error: input file is too short (2 bytes) to replace 8 bytes; use 'insert' instead
error: refusing to overwrite the input file; choose a different -o/--output path or pass --force
error: unknown preset 'bmp'; valid presets: png, gif89a, gif87a, jpeg, pdf, zip, elf, pe, gzip, 7z, rar, javaclass
error: could not recognize the current file's signature, so there is no safe splice point for 'replace'; pass raw=True (CLI: --raw) to force a fixed-length overwrite, or use 'insert' to just prepend bytes
```

---

## Known limitations

- **Signature-boundary detection only recognizes the signatures in the
  detection table** (`go/internal/mbyte/core.go`). For any other format,
  `replace` can't find a safe splice point and requires `--raw`.
- **JPEG detection matches the generic 3-byte `FF D8 FF` prefix.** The 4th byte
  (JFIF `E0` vs EXIF `E1`, etc.) is treated as part of the trailing data, not
  the signature, so a `replace` onto a JPEG may not touch that 4th byte. (The
  `jpeg` preset itself still writes the full 4-byte `ffd8ffe0` on insert.)
- **BMP は検出専用** で、insert プリセットとしては公開していません。

---

## 開発・テスト

Go 実装（主）：

```bash
cd go
go test ./... -v
```

Python 参照実装（`legacy-python/`）：

```bash
cd legacy-python
pip install -e ".[test]"
pytest -q
```

CI（GitHub Actions）で Go と Python の両方をビルド・テストします。

---

## 懸念点・注意事項

- **フォーマット判定は「先頭バイトの推測」にすぎません。**
  `detect_format` は先頭のシグネチャを見て名前を返すだけで、ファイル全体が正当な
  その形式であることを保証しません。本物の `file(1)` とは判定が一致しないこともあります。
  実際にパースできるかは `mbyte check` で確認できます（それでも完全な検証ではありません）。

- **`insert` は正規のフォーマットとしては壊れたファイルを作ります。**
  先頭にバイト列を足すだけなので、元データのオフセットがずれます。ブラウザや
  画像ビューアが「正しい画像」として開けるとは限りません。先頭バイトしか見ない検証にのみ有効です。

- **`replace` は元の先頭バイトを失います。**
  上書きした分の元データは出力には残りません（元の入力ファイルは変更しませんが、出力からは消えます）。

- **これは検証回避の万能ツールではありません。**
  Content-Type チェック、拡張子チェック、再エンコード、画像の再描画、実際のパースを伴う
  検証などはマジックバイト偽装だけでは通りません。あくまで「先頭シグネチャだけを見る」
  タイプの検証を学ぶためのものです。

- **ウイルス対策ソフトが介入することがあります。**
  Web シェル文字列（例: `<?php system($_GET["cmd"]); ?>`）を含むファイルを作ると、
  AV が検知してファイルを隔離し、読み書き時にエラーになる場合があります。
  これはツールのバグではありません。検証用フォルダを AV の除外対象にするか、無害なデータで
  動作確認してください。

- **改行コードについて。**
  本ツールはバイト列をそのまま扱いますが、シェルで `echo` などを使ってテキスト入力を作ると、
  OS によって改行が LF / CRLF に変わり、結果のバイト列が環境で異なることがあります。

---

## 免責事項

本ツールは **Capture The Flag（CTF）の練習**、および **明示的な許可を得た環境** での
セキュリティ検証に使うことのみを目的としています。自分が所有していない、または検証の許可を
得ていないシステムに対してファイルアップロード制御を回避する目的で使用することは、
違法となる可能性があります。作者は悪用を推奨・是認せず、悪用により生じたいかなる結果についても
責任を負いません。責任を持って利用してください。

---

## ライセンス

MIT — 詳細は [LICENSE](LICENSE) を参照してください。
