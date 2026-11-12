# mbyte Go移植 + 機能拡張 実装ステップ指示書

対応するUser Story / MoSCoWは `01-user-stories-moscow.md` を参照。
このファイルは「次にこれをやる」の実行順の指示書として使う(Claude Codeへの指示、または自分で実装する際のチェックリストとして使える粒度で書いている)。

各ステップは「目的 / やること / 完了条件(DoD) / 参考」の形式。
前のステップが終わってから次に進むこと(特にStep 1〜3は依存関係がある)。

---

## Step 0: Go環境セットアップ

**目的**: 移植先のGoプロジェクトの骨格を作る。

**やること**:
1. リポジトリ内に `go/` ディレクトリを作る(移行期間中はPython版と並存させ、移行完了後にルートへ昇格する)
2. `go mod init github.com/<akira-aのGitHubユーザー名>/mbyte` でモジュール初期化
3. ディレクトリ構成を決める:
   ```
   go/
     cmd/mbyte/main.go        # エントリポイント
     internal/mbyte/core.go   # 署名定義・hex解析・insert/replace
     internal/mbyte/detect.go # 署名検出
     internal/mbyte/batch.go  # バッチ処理(Step 5で追加)
     internal/mbyte/verify.go # ハッシュ比較・polyglot検証(Step 3,6で追加)
     internal/mbyte/core_test.go
   ```
4. CLIライブラリを選定する。候補: 標準`flag`+手書きサブコマンド分岐(軽量) か `spf13/cobra`(機能豊富、補完生成が楽)。
   → **推奨: cobra**。shell補完(`mbyte completion bash`)が標準機能として手に入り、Step準備で話した「Kaliツールらしさ」(補完対応)をほぼ無料で満たせる。

**完了条件**: `go build ./...` が空実装でも通る。`go.mod`/`go.sum`がコミットされている。

**参考**: `go.dev/doc/modules/layout`

---

## Step 1: coreロジックの移植(US-0.1)

**目的**: Python `core.py` の以下をGoで再現する。
- `parse_hex` (hex文字列の検証+バイト変換)
- `_SIGNATURES` (署名テーブル、longest-match-wins)
- `detect_format` / `detect_signature_length`
- `insert_magic_bytes`
- `replace_magic_bytes`(`raw`有無の両方、境界検出ロジック含む)
- `hexdump`

**やること**:
1. `_SIGNATURES`を `[]Signature{ {Hex: []byte{...}, Name: "PNG image data"}, ... }` のようなスライスとして定義する。JPEGの3バイト一致の挙動(README記載の「4バイト目はマーカー境界」の既知の制約)もコメントごと引き継ぐ。
2. `ParseHex(s string) ([]byte, error)` — 奇数桁・不正文字はエラーを返す(Pythonの`MbyteError`相当は独自エラー型 `var ErrInvalidHex = errors.New(...)` などで表現)。
3. `MatchSignature(data []byte) (sig []byte, name string, ok bool)` — longest-match-wins。
4. `DetectFormat`/`DetectSignatureLength` をこの上に実装。
5. `InsertMagicBytes(inputPath, outputPath string, magic []byte) error` — prependして書き込み。
6. `ReplaceMagicBytes(inputPath, outputPath string, magic []byte, raw bool) error` — Python版と同じ分岐(raw=trueなら固定長上書き、falseなら境界検出)。
7. `Hexdump(data []byte, length int) string` — xxd風1行出力(Python版と同じフォーマット)。

**完了条件**:
- Step 7(テスト移植)で、Python版`tests/test_core.py`の全ケースに対応するGoテストが**全部green**。
- 既存のサンプルファイル(`shell.php`, `shell.png.php`)に対してPython版・Go版で**バイト単位で同一の出力**が得られることを手動で1回diffして確認する。

**参考**: 既存 `mbyte/core.py` を1対1で読みながら移植する。ロジックを「改善」するのはこの時点ではやらない(移植の正しさを先に確定させる)。

---

## Step 2: CLI層の移植(US-0.2)

**目的**: `cli.py` のサブコマンド構成をGoで再現する。

**やること**:
1. cobraで `insert` / `replace` サブコマンドを作る。
2. フラグ: `--preset`(string) / `--hex`(string) は相互排他(cobraの`MarkFlagsMutuallyExclusive`を使う)。
3. `-o`/`--output`、`--force`、`-v`/`--verbose`、`replace`限定の`--raw`を再現。
4. `--list-presets` と `--version` をルートコマンドのフラグとして再現。
5. 出力先解決ロジック(`_resolve_output`相当): presetなら拡張子マップ、未指定なら`.mbyte`拡張子。入力と出力が同一パスなら`--force`無しでエラー。
6. 実行後の出力(hexdump + detected format)をPython版と同じ見た目で出す。

**完了条件**: READMEに書かれている全コマンド例を実行し、Python版と見た目(hexdumpの桁・detected formatの文言含む)が一致する。

**参考**: `go/cmd/mbyte/main.go` に薄く置き、ロジックは全部`internal/mbyte`パッケージ側に置く(CLI層にロジックを書かない。Pythonの`core.py`/`cli.py`分離を維持する)。

---

## Step 3: シグネチャプリセット拡充 + 一本化(US-1.1, US-1.2)

**目的**: 検出リストとプリセットリストの非対称を解消する。

**やること**:
1. Step 1で作った署名テーブル(`[]Signature`)に、insert時に使う「プリセット名」と「デフォルト出力拡張子」を持たせる。例:
   ```go
   type Signature struct {
       Hex        []byte
       Name       string // detect_formatが返す名前
       PresetName string // "" なら insert プリセットとしては未公開(検出専用)
       Ext        string // PresetNameがある場合の既定出力拡張子
   }
   ```
2. 既存6種(png/gif89a/gif87a/jpeg/pdf/zip)に加え、検出済みの ELF / PE32 / gzip / 7-zip / RAR / Javaクラス にも `PresetName`/`Ext` を付与する。
3. `--list-presets` は `PresetName != ""` のものだけを列挙する(検出専用シグネチャは混ぜない)。
4. `_resolve_hex`相当のロジックを、この単一テーブルから検索するように書き換える(Pythonの`PRESETS`辞書は廃止し、テーブル1本に統一)。

**完了条件**: `mbyte --list-presets` が14種類前後を表示し、そのうち新規追加分(elf/pe/gzip/7z/rar/javaclass)全てで`mbyte insert --preset <name> payload.bin -o out`が成功し、`detect_format`で正しく検出し直せる。

**Should/Could**: US-1.3(外部シグネチャファイル)は、この単一テーブルをJSONにシリアライズできる形にしておけば後から追加しやすい。今回は設計だけ意識し、実装は見送って良い。

---

## Step 4: ペイロードハッシュ比較(US-2.2) + diffプレビュー(US-2.1)

**目的**: 「ペイロードが無傷であること」を客観的に保証し、実行前に変化を可視化する。

**やること**:
1. `HashPayload(data []byte, boundary int) string` — `data[boundary:]`のSHA-256を16進文字列で返す(`crypto/sha256`)。
2. `insert`の場合: boundary=0相当(全データがそのままペイロード)なので、操作前後で元ファイル全体のハッシュが一致することを確認して表示。
3. `replace`の場合:
   - 操作前: 検出した境界(`DetectSignatureLength`)以降のハッシュを計算
   - 操作後: 新しい境界(新署名の長さ)以降のハッシュを計算
   - 両者が一致すれば `[MATCH]`、不一致(`--raw`で長さがズレた等)なら `[MISMATCH]` + 何バイトズレたかを表示
4. 実行前プレビュー: 書き込み前に以下を標準出力(または`-y`/`--yes`が無ければ確認プロンプト、`--dry-run`ならここで終了):
   ```
   [preview] input  : upload.dat (12345 bytes)
   [preview] current signature : (unrecognized) / detected as "GIF image data, version 89a" (6 bytes)
   [preview] new     signature : ffd8ffe0 (JPEG image data, 4 bytes)
   [preview] payload hash (before): 9f86d0...
   [preview] estimated payload hash (after): 9f86d0... [MATCH]
   [preview] output : upload.jpg (12343 bytes, -2 bytes)
   ```
5. `--yes`/`-y`フラグで対話確認をスキップ(バッチ処理やスクリプトからの利用を想定)。`--dry-run`は書き込みをせずプレビューのみで終了。

**完了条件**: `replace`で境界検出が効くケース・`--raw`で長さがズレるケースの両方で、MATCH/MISMATCHが正しく表示されることをテストで確認。

---

## Step 5: バッチ処理(US-3.1, Could群のUS-3.2/3.3)

**目的**: 複数ファイルへの一括適用。

**やること**:
1. `insert`/`replace`の第一引数(`input_file`)がディレクトリの場合、中の全ファイル(再帰はデフォルトOFF、`--recursive`で有効化)に同じ操作を適用する分岐を追加。
2. 出力先はデフォルトで `<output指定ディレクトリ>/<元のファイル名>.<拡張子>`。
3. 各ファイルの結果(成功/失敗、検出フォーマット、payload hash一致/不一致)を配列で集めて、最後に表形式で一覧表示。
4. `--json`フラグで、同じ結果をJSON配列として標準出力に出す(スクリプトからの利用・他ツールへのパイプを想定、US-3.3)。
5. (Could, US-3.2) `--all-presets`フラグ: 単一ファイルに対して、Step3で定義した全プリセットを適用し、`<output-dir>/<元ファイル名>.<各拡張子>` を一括生成するモード。

**完了条件**: サンプルディレクトリ(複数ファイル)に対して一括実行し、サマリ表示・JSON出力の両方が正しいこと。

---

## Step 6: polyglot構造検証(US-4.1, US-4.2)

**目的**: 「署名が一致するだけ」と「実際にそのフォーマットとしてパースできる」の違いを検証・提示する。Go標準ライブラリのデコーダを使えるのがPython版からの最大の進化点。

**やること**:
1. 主要4フォーマットから着手(優先度順):
   - PNG: `image/png` の `png.Decode` / `png.DecodeConfig` でデコードを試す
   - JPEG: `image/jpeg` の `jpeg.Decode`
   - GIF: `image/gif` の `gif.Decode`
   - ZIP: `archive/zip` の `zip.NewReader` (末尾のEnd of Central Directoryも見るのでシグネチャ偽装だけでは通らないことが実際に確認できる好例)
2. `CheckStructuralValidity(path string, formatHint string) (ok bool, detail string)` を実装。デコードが成功すれば`ok=true`、失敗ならそのエラー内容(`detail`)をそのまま返す(「どこで落ちたか」が学習的価値になる)。
3. ELF/PEは `debug/elf` / `debug/pe` で同様に試す(こちらはヘッダの整合性までしか見ないことが多いが、シグネチャのみ偽装したファイルとの違いは十分出る)。
4. `mbyte check <file>` サブコマンドを新設(US-4.2): 署名一致結果(`detect_format`)と構造妥当性結果(`CheckStructuralValidity`)の両方を1つのレポートとして出力する。
5. `insert`/`replace`実行後にも、対応フォーマットがあれば自動でこのチェックを走らせ、結果を末尾に追記する(`--no-check`で無効化可能にする)。

**完了条件**: `insert --preset png`しただけ(=本物のPNGチャンク構造を持たない)ファイルに対して `mbyte check` を実行すると、「署名: PNG一致」「構造: FAIL(png: invalid format: invalid checksum 等)」のように**意図的に失敗する**ことを確認できる。これが「先頭バイトだけの検証」と「実パース検証」の違いを示す動く実例になる。

---

## Step 7: テスト移植(US-0.3)

**目的**: リグレッション防止。

**やること**:
1. `tests/test_core.py`の各テストケースをGoの`internal/mbyte/core_test.go`に1対1で移植(正常系: insert/replace、異常系: 不正hex/short replace/ファイル不存在/プリセット不明/署名未検出)。
2. Step3〜6で追加した機能ごとにテストを追加(プリセット拡充、ハッシュ比較のMATCH/MISMATCH、バッチ処理、polyglot検証の成功/失敗両方)。
3. `go test ./...` をCI(GitHub Actions等)に組み込む。

**完了条件**: `go test ./... -v` が全てpass。

---

## Step 8: ビルド・配布

**目的**: 「bashからすぐ呼べる」状態への最終仕上げ。

**やること**:
1. `goreleaser`導入(複数OS/アーキテクチャ向けのビルド+GitHub Releases自動化)。
2. `README.md`をGo版の使い方に合わせて更新(インストール手順を`go install`/バイナリダウンロードに変更)。
3. shell補完生成コマンド(`mbyte completion bash > /etc/bash_completion.d/mbyte`等)をREADMEに追記。
4. Python版は`legacy-python/`などに移すか、タグを打って区切ってから削除する(どちらにするかは実装時に判断)。

**完了条件**: タグ付きリリースから各OS向けバイナリがダウンロードでき、PATHに置くだけで`mbyte --list-presets`が動く。

---

## 進行管理の注意

- Step 1〜3は**この順番を守る**(署名テーブルの設計がStep4〜6の土台になるため)。
- Step 4(ハッシュ比較)とStep 6(polyglot検証)は目的が違う: 4は「元データが変わっていないか」、6は「変換後のデータが対象フォーマットとして正しいか」。両方実装して初めて「このツールは何を保証していて、何を保証していないか」をユーザーに正確に伝えられる。
- 各StepのDoDを満たしてから次のStepに進むこと。特にStep1はテストなしで次に進まない。
