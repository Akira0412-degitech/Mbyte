# mbyte — Magic Byte Insert/Replace CLI

`mbyte` は、ファイル先頭のマジックバイト（ファイルシグネチャ）を **挿入（insert）** または
**置換（replace）** するための、依存ライブラリなしの小さな Python 製 CLI ツールです。

ファイルアップロード時のファイル種別検証がどのように行われ、どのように回避されうるか
（マジックバイト偽装）を **CTF や、許可された環境でのセキュリティ検証** を通じて学ぶための
教育用ツールです。

やっていること自体は単純で、「先頭にバイト列を足す」か「先頭の N バイトを上書きする」だけです。
価値は、その単純な操作を **安全に・わかりやすく** 行える点にあります — 名前付きプリセット、
入力バリデーション、処理後に `file` / `xxd` 相当の結果を表示する確認機能、を備えています。

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

ローカル（開発）インストール（editable）：

```bash
pip install -e .
```

これで `mbyte` コマンドが使えるようになります。
（将来 PyPI に公開した場合は `pip install mbyte` を想定しています。）

- 必要環境: **Python 3.8 以上**
- 実行時の第三者依存ライブラリ: **なし**（標準ライブラリのみ）
- テスト用に pytest を入れる場合: `pip install -e ".[test]"`

---

## 使い方

```
mbyte insert  --preset <name> <input_file> -o <output_file>
mbyte insert  --hex <hexstring> <input_file> -o <output_file>
mbyte replace --preset <name> <input_file> -o <output_file>
mbyte replace --hex <hexstring> <input_file> -o <output_file>
mbyte --list-presets
```

### オプション

| オプション | 説明 |
| ---------- | ---- |
| `--preset <name>` | 名前付きプリセットを使う（`--list-presets` で一覧） |
| `--hex <hexstring>` | 生の 16 進文字列を直接指定（例: `89504e470d0a1a0a`） |
| `-o`, `--output` | 出力ファイルパス。**省略時は `<input_file>.mbyte`** |
| `--force` | 入力ファイル自体への上書きを許可（既定では事故防止のため禁止） |
| `-v`, `--verbose` | 詳細ログを表示 |
| `--list-presets` | プリセット一覧を表示して終了 |
| `--version` | バージョンを表示 |

- `--preset` と `--hex` は **どちらか一方が必須**（両方指定はエラー）。
- 出力先が入力ファイルと同一になる場合、`--force` がないと処理を拒否します（上書き事故防止）。

### 実行例

プリセットを使って PNG ヘッダを先頭に追加：

```bash
mbyte insert --preset png payload.bin -o payload.png
```

生の 16 進で任意のシグネチャを追加：

```bash
mbyte insert --hex 89504e470d0a1a0a notes.txt -o notes.png.txt
```

先頭バイトを JPEG シグネチャで置換：

```bash
mbyte replace --preset jpeg upload.dat -o upload.jpg
mbyte replace --hex ffd8ffe0 upload.dat -o upload.jpg
```

成功すると、判定されたフォーマットと先頭 16 バイトの hexdump が表示されます：

```
[+] wrote payload.png
[+] detected format: PNG image data
00000000: 89 50 4e 47 0d 0a 1a 0a 70 61 79 6c 6f 61 64 00  .PNG....payload.
```

`-v` を付けると、コマンド種別・入出力パス・使用した 16 進列も表示されます。

---

## プリセット一覧

| 名前     | Hex                | フォーマット              |
| -------- | ------------------ | ------------------------- |
| `png`    | `89504e470d0a1a0a` | PNG 画像                  |
| `gif89a` | `474946383961`     | GIF 画像 (version 89a)    |
| `gif87a` | `474946383761`     | GIF 画像 (version 87a)    |
| `jpeg`   | `ffd8ffe0`         | JPEG 画像                 |
| `pdf`    | `25504446`         | PDF ドキュメント          |
| `zip`    | `504b0304`         | ZIP アーカイブ            |

`mbyte --list-presets` で CLI からも確認できます。

---

## 終了コードとエラー

| 状況 | 終了コード | 挙動 |
| ---- | ---------- | ---- |
| 正常終了 | `0` | 出力ファイルを書き込み、確認情報を表示 |
| 実行時エラー | `1` | 不正な 16 進・ファイル不存在・short replace・上書き拒否・未知プリセット など。`error: ...` を標準エラーに表示 |
| 引数エラー | `2` | argparse による引数自体の誤り（例: `--preset` と `--hex` の同時指定） |

主なエラーメッセージの例：

```
error: hex string has an odd number of digits (3): 'xyz'
error: input file is too short (2 bytes) to replace 8 bytes; use 'insert' instead
error: refusing to overwrite the input file; choose a different -o/--output path or pass --force
error: unknown preset 'bmp'; valid presets: png, gif89a, gif87a, jpeg, pdf, zip
```

---

## 開発・テスト

```bash
pip install -e ".[test]"
pytest -q
```

テストは `tests/test_core.py` にあり、正常系（insert/replace）と異常系
（不正 hex、short replace、ファイル不存在、プリセット値、フォーマット判定）をカバーしています。

---

## 懸念点・注意事項

- **フォーマット判定は「先頭バイトの推測」にすぎません。**
  `detect_format` は先頭のシグネチャを見て名前を返すだけで、ファイル全体が正当な
  その形式であることを保証しません。本物の `file(1)` とは判定が一致しないこともあります。

- **`insert` は正規のフォーマットとしては壊れたファイルを作ります。**
  先頭にバイト列を足すだけなので、元データのオフセットがずれます。ブラウザや
  画像ビューアが「正しい画像」として開けるとは限りません。先頭バイトしか見ない検証にのみ有効です。

- **`replace` は元の先頭バイトを失います。**
  上書きした分の元データは出力には残りません（元の入力ファイルは変更しませんが、出力からは消えます）。

- **これは検証回避の万能ツールではありません。**
  Content-Type チェック、拡張子チェック、再エンコード、画像の再描画、実際のパースを伴う
  検証などはマジックバイト偽装だけでは通りません。あくまで「先頭シグネチャだけを見る」
  タイプの検証を学ぶためのものです。

- **Windows Defender などのウイルス対策ソフトが介入することがあります。**
  Web シェル文字列（例: `<?php system($_GET["cmd"]); ?>`）を含むファイルを作ると、
  AV が検知してファイルを隔離し、読み書き時に `PermissionError`（Errno 13）になる場合があります。
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
