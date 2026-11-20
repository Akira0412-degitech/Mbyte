# mbyte — Magic Byte Insert/Replace CLI

**English** | [日本語](README.ja.md)

`mbyte` is a small **Go** CLI tool that **inserts** or **replaces** the magic bytes
(the file signature) at the start of a file.

It is an educational tool for learning — through **CTF challenges and authorized security
testing** — how file-type validation on uploads works and how it can be bypassed (magic-byte
spoofing).

What it does is simple: it either prepends a byte sequence or overwrites the first N bytes.
The value is in doing that simple operation **safely and legibly** — named presets, input
validation, a post-op confirmation that prints `file` / `xxd`-style output, a SHA-256 comparison
that proves the payload is intact, and a structural check (`check`) that shows the difference
between "the signature matches" and "it actually parses as that format".

> This repository was originally written in Python and has been ported to Go. The original Python
> implementation is kept for reference in [`legacy-python/`](legacy-python/) (it has not been deleted).

---

## insert vs. replace

These two look similar but produce completely different results. Pick the one that fits your goal.

| Subcommand | Behavior | File size | Main use |
| ---------- | -------- | --------- | -------- |
| `insert`  | **Prepends** the given bytes at the start (keeps all original content) | grows | Bypassing loose checks where the content is irrelevant as long as the signature passes |
| `replace` | **Overwrites** the first N bytes with the given bytes (keeps the rest) | unchanged | Bypassing checks that only inspect the first few bytes |

- `insert` does not corrupt the original data, so even if an image viewer treats it as a broken
  image, the original content is still readable.
- `replace` errors out if the original file is shorter than the given byte sequence (use `insert`).

---

## Installation

### Download a release binary (recommended / no Go needed)

[GitHub Releases](https://github.com/Akira0412-degitech/Mbyte/releases) provides archives for each
OS / architecture. Names are `mbyte_<version>_<os>_<arch>.tar.gz` (Linux / macOS) or `.zip`
(Windows), and a `checksums.txt` (SHA-256) is included.

| OS | amd64 | arm64 |
| -- | ----- | ----- |
| Linux (e.g. Kali) | `mbyte_0.1.0_linux_amd64.tar.gz` | `mbyte_0.1.0_linux_arm64.tar.gz` |
| macOS | `mbyte_0.1.0_darwin_amd64.tar.gz` | `mbyte_0.1.0_darwin_arm64.tar.gz` |
| Windows | `mbyte_0.1.0_windows_amd64.zip` | `mbyte_0.1.0_windows_arm64.zip` |

Kali / Linux (amd64) example — download → verify checksum → extract → put on PATH:

```bash
VER=0.1.0
BASE="https://github.com/Akira0412-degitech/Mbyte/releases/download/v${VER}"

# Fetch the binary and the checksums
curl -fsSLO "${BASE}/mbyte_${VER}_linux_amd64.tar.gz"
curl -fsSLO "${BASE}/checksums.txt"

# Verify the SHA-256 (prints "mbyte_0.1.0_linux_amd64.tar.gz: OK")
sha256sum --ignore-missing -c checksums.txt

# Extract and move it onto your PATH
tar xzf "mbyte_${VER}_linux_amd64.tar.gz"
sudo install -m 0755 mbyte /usr/local/bin/mbyte
mbyte --version
```

On Windows, extract the `.zip` and put `mbyte.exe` on your PATH (verify with PowerShell's
`Get-FileHash mbyte_0.1.0_windows_amd64.zip -Algorithm SHA256` against `checksums.txt`).

### `go install` (if you have the Go toolchain)

```bash
go install github.com/Akira0412-degitech/Mbyte/cmd/mbyte@latest
```

The `mbyte` binary lands in `$(go env GOPATH)/bin` (put it on your PATH).

### Build from source

```bash
git clone https://github.com/Akira0412-degitech/Mbyte.git
cd Mbyte/go
go build -o mbyte ./cmd/mbyte
```

- Requires: **Go 1.21 or newer**
- Dependencies: [cobra](https://github.com/spf13/cobra) for the CLI (see `go.mod`).

---

## Usage

```
mbyte insert  --preset <name> <input> -o <output>
mbyte insert  --hex <hexstring> <input> -o <output>
mbyte replace --preset <name> <input> -o <output>
mbyte replace --hex <hexstring> <input> -o <output>
mbyte check   <file>
mbyte --list-presets
```

### Options (`insert` / `replace`)

| Option | Description |
| ------ | ----------- |
| `--preset <name>` | Use a named preset (list them with `--list-presets`) |
| `--hex <hexstring>` | Give a raw hex string directly (e.g. `89504e470d0a1a0a`) |
| `-o`, `--output` | Output file path. **Defaults to `<input>.mbyte`** (with a preset, that preset's default extension) |
| `--force` | Allow overwriting the input file itself (disabled by default to prevent accidents) |
| `--raw` | (`replace` only) Skip signature-boundary detection and overwrite exactly `len(hex)` bytes from the start (the old, unsafe behavior) |
| `-y`, `--yes` | Skip the pre-write confirmation prompt |
| `--dry-run` | Show the preview and exit without writing |
| `--no-check` | Do not run the post-write structural (decode) check |
| `-r`, `--recursive` | When the input is a directory, also recurse into subdirectories (batch) |
| `--json` | When the input is a directory, print results as a JSON array |
| `-v`, `--verbose` | Print detailed logs |
| `--list-presets` | Print the preset list and exit (root) |
| `--version` | Print the version (root) |

- Exactly **one of `--preset` or `--hex` is required** (giving both is an error).
- If the output path would equal the input file, the operation is refused without `--force`
  (to prevent accidental overwrites).
- The **confirmation prompt** only appears when running interactively from a terminal (TTY).
  Non-interactive runs (pipes, scripts, etc.) proceed as-is (equivalent to `-y`). Directory batch
  runs are also non-interactive.

### Examples

The following use the bundled sample `shell.php` (a harmless text file containing a web-shell
string) and are copy-paste runnable. Run them from the repository root right after cloning (if
`mbyte` is not on your PATH, give a path such as `./go/mbyte`).

Prepend a PNG header with a preset:

```bash
mbyte insert --preset png shell.php -o demo.png
```

On success, the detected format and a hexdump of the first 16 bytes go to stdout, and the
structural-check result goes to stderr:

```
[+] wrote demo.png
[+] detected format: PNG image data
00000000: 89 50 4e 47 0d 0a 1a 0a ff fe 3c 00 3f 00 70 00  .PNG......<.?.p.
[check] structure: FAIL (png: invalid format: Bad chunk length: 4294851584)
```

> That `[check] structure: FAIL` is intentional. It shows that merely attaching a PNG signature
> does not make a valid PNG (see "Structural check" below).

Prepend an arbitrary signature as raw hex:

```bash
mbyte insert --hex 89504e470d0a1a0a shell.php -o demo.png.php
```

Replace the leading bytes with a JPEG signature (`demo.png` is the file from the example above):

```bash
mbyte replace --preset jpeg demo.png -o demo.jpg
```

Preview the change and the payload hash before writing (no write):

```bash
mbyte replace --preset jpeg demo.png -o demo.jpg --dry-run
```

```
[preview] input    : demo.png (74 bytes)
[preview] current signature : 89504e470d0a1a0a ("PNG image data", 8 bytes)
[preview] new signature     : ffd8ffe0 ("JPEG image data", 4 bytes)
[preview] payload hash (before)           : 69009c...
[preview] payload hash (after, estimated) : 69009c... [MATCH]
[preview] output   : demo.jpg (70 bytes, -4 bytes)
[*] dry-run: no file written
```

Run without the confirmation prompt (for scripts):

```bash
mbyte insert --preset png shell.php -o demo.png -y
```

Apply to every file in a directory (recursive, results as JSON):

```bash
mbyte insert --preset png ./some_dir -o ./out --recursive --json
```

### `replace`: signature-boundary detection and `--raw`

By default, `replace` detects the length of the file's **existing** signature and splices the new
bytes in at that boundary. It works whether the new signature is shorter or longer than the old one
(because the cut point is "where the original signature actually ends", not the length of the new
signature). This avoids leftover garbage bytes (when the new signature is shorter) and corruption of
real data (when it is longer).

```bash
# Safe (default): detect the existing signature's real length and splice there
mbyte replace --preset png broken.png -o fixed.png

# Manual / dangerous: skip detection and overwrite len(hex) bytes from the start
mbyte replace --preset png broken.png -o fixed.png --raw
```

If the current signature is not recognized, `replace` (without `--raw`) errors out rather than
guessing the splice point. Force a fixed-length overwrite with `--raw`, or use `insert` if you just
want to prepend bytes.

---

## Payload integrity check

`insert` / `replace` compare the SHA-256 of the **payload** (the bytes after the signature) before
and after the operation, showing `[MATCH]` / `[MISMATCH]` / `[UNVERIFIED]` (displayed in the preview
with `--dry-run`, `-v`, or during an interactive run).

- `insert` and boundary-detecting `replace` preserve the payload, so they are always `[MATCH]`.
- When `--raw` shifts the length, it shows `[MISMATCH] (payload shifted by ±N bytes)` with the exact
  byte offset.

---

## Structural check (`check`)

`check` combines `detect_format` (the leading-signature guess) with an **actual parse** using the Go
standard library's decoders, in a single report. It reveals the difference between "the signature
matches" and "it is genuinely valid as that format".

Try it with the bundled `shell.png.php` (a PNG-signatured sample produced by
`insert --preset png shell.php`):

```bash
mbyte check shell.png.php
```

```
file: shell.png.php
[signature] PNG image data (matched 8 bytes)
[structure] FAIL (png: invalid format: Bad chunk length: 4294851584)
```

- Supported formats: PNG / JPEG / GIF (`image/*`), ZIP (`archive/zip`), ELF / PE (`debug/elf` /
  `debug/pe`).
- A valid structure prints `[structure] PASS`; unsupported formats print `[structure] SKIP`.
- On a structural FAIL, `check` returns exit code `1`.
- `insert` / `replace` also run the same check automatically after writing and append the result to
  stderr (disable with `--no-check`).

ZIP validates the End of Central Directory at the end of the file, so merely prepending a signature
can never make it valid — a vivid example of why "first-bytes-only" spoofing can't pass a real check.

---

## Preset list

| Name        | Hex                | Format                    |
| ----------- | ------------------ | ------------------------- |
| `png`       | `89504e470d0a1a0a` | PNG image                 |
| `gif89a`    | `474946383961`     | GIF image (version 89a)   |
| `gif87a`    | `474946383761`     | GIF image (version 87a)   |
| `jpeg`      | `ffd8ffe0`         | JPEG image                |
| `pdf`       | `25504446`         | PDF document              |
| `zip`       | `504b0304`         | ZIP archive               |
| `elf`       | `7f454c46`         | ELF executable            |
| `pe`        | `4d5a`             | PE32 executable (MZ)      |
| `gzip`      | `1f8b`             | gzip compressed data      |
| `7z`        | `377abcaf271c`     | 7-zip archive             |
| `rar`       | `52617221`         | RAR archive               |
| `javaclass` | `cafebabe`         | Java class file           |

You can also list these from the CLI with `mbyte --list-presets`.

---

## Shell completion

cobra's completion generation is available (bash / zsh / fish / powershell). Example (bash):

```bash
mbyte completion bash > /etc/bash_completion.d/mbyte      # system-wide
# or
mbyte completion bash > ~/.local/share/bash-completion/completions/mbyte
```

See `mbyte completion --help` for per-shell instructions.

---

## Exit codes and errors

| Situation | Exit code | Behavior |
| --------- | --------- | -------- |
| Success | `0` | Writes the output file and prints confirmation info |
| Runtime error | `1` | Invalid hex, missing file, short replace, overwrite refused, unknown preset, a failure within a batch, or a structural FAIL from `check`, etc. Prints `error: ...` to stderr (batch / `check` only use the exit code since they already printed their results) |
| Argument error | `2` | A mistake in the arguments themselves (e.g. `--preset` and `--hex` together, or an unknown flag) |

Common error messages:

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
- **BMP is detection-only** and is not exposed as an insert preset.

> **Improvement in the Go version:** the original Python version only did the "leading-signature
> guess" and could not confirm whether a file actually decodes as that format. The Go version adds a
> **structural check** (the `check` subcommand and the automatic check after insert / replace) using
> standard-library decoders, so the gap between "the signature matches" and "it genuinely parses"
> can be demonstrated. Note that this is still not a complete validity check — unsupported formats
> become `SKIP`.

---

## Development & testing

Go implementation (primary):

```bash
cd go
go test ./... -v
```

Python reference implementation (`legacy-python/`):

```bash
cd legacy-python
pip install -e ".[test]"
pytest -q
```

CI (GitHub Actions) builds and tests both Go and Python.

---

## Caveats & notes

- **Format detection is only a "first-bytes guess".**
  `detect_format` just looks at the leading signature and returns a name; it does not guarantee that
  the whole file is a valid file of that format. Its verdict may not match real `file(1)`. Whether it
  actually parses can be checked with `mbyte check` (and even that is not a complete validation).

- **`insert` produces a file that is broken as a real format.**
  It only prepends bytes, so the original data's offsets shift. A browser or image viewer will not
  necessarily open it as a "correct image". It is only effective against checks that look at the
  first bytes.

- **`replace` loses the original leading bytes.**
  The overwritten portion of the original data is not kept in the output (the original input file is
  not modified, but it is gone from the output).

- **This is not a universal bypass tool.**
  Content-Type checks, extension checks, re-encoding, image re-rendering, and validation that
  actually parses the file cannot be passed with magic-byte spoofing alone. It is only for learning
  about the "inspect the leading signature only" kind of validation.

- **Antivirus may interfere.**
  If you create a file containing a web-shell string (e.g. `<?php system($_GET["cmd"]); ?>`),
  AV may detect and quarantine it, causing read/write errors. This is not a bug in the tool. Exclude
  your test folder from AV, or verify behavior with harmless data.

- **About newlines.**
  This tool handles bytes as-is, but if you create text input using shell `echo` and similar, the OS
  may turn newlines into LF / CRLF, so the resulting bytes can differ between environments.

---

## Disclaimer

This tool is intended **only** for **Capture The Flag (CTF) practice** and security testing in
**environments where you have explicit permission**. Using it to bypass file-upload controls on
systems you do not own or are not authorized to test may be illegal. The author does not encourage or
condone misuse and is not responsible for any consequences arising from misuse. Use it responsibly.

---

## License

MIT — see [LICENSE](LICENSE) for details.
