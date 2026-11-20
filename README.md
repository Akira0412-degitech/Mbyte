# mbyte — Magic Byte Insert/Replace CLI

**English** | [日本語](README.ja.md)

`mbyte` is a small **Go** CLI that **inserts** or **replaces** the magic bytes (the file
signature) at the start of a file. It is an educational tool for learning — in **CTF challenges and
authorized security testing** — how upload file-type checks work and how magic-byte spoofing can
bypass them.

It does one simple thing (prepend bytes, or overwrite the first N bytes) but does it **safely**:
named presets, input validation, a SHA-256 payload check, and a structural check (`check`) that
shows the gap between "the signature matches" and "it actually parses as that format".

> Originally written in Python, now ported to Go. The Python version is kept for reference in
> [`legacy-python/`](legacy-python/).

---

## insert vs. replace

| Subcommand | Behavior | File size | Use for |
| ---------- | -------- | --------- | ------- |
| `insert`  | Prepends the bytes (keeps all original content) | grows | Checks that pass as long as the signature matches |
| `replace` | Overwrites the first N bytes (keeps the rest) | unchanged | Checks that only inspect the first few bytes |

`insert` never corrupts the original data; `replace` errors out if the file is shorter than the new
bytes (use `insert` instead).

---

## Installation

### Release binary (recommended, no Go needed)

Download from [GitHub Releases](https://github.com/Akira0412-degitech/Mbyte/releases). Archives are
named `mbyte_<version>_<os>_<arch>.tar.gz` (Linux / macOS) or `.zip` (Windows), with a
`checksums.txt` (SHA-256). Linux / macOS / Windows × amd64 / arm64 are provided.

Linux (amd64) example:

```bash
VER=0.2.0
BASE="https://github.com/Akira0412-degitech/Mbyte/releases/download/v${VER}"
curl -fsSLO "${BASE}/mbyte_${VER}_linux_amd64.tar.gz"
curl -fsSLO "${BASE}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt      # verify
tar xzf "mbyte_${VER}_linux_amd64.tar.gz"
sudo install -m 0755 mbyte /usr/local/bin/mbyte
```

On Windows, extract the `.zip` and put `mbyte.exe` on your PATH (verify with
`Get-FileHash ... -Algorithm SHA256` against `checksums.txt`). Once installed, `mbyte update` keeps
it current (see [Self-update](#self-update-update)).

### go install / from source

```bash
go install github.com/Akira0412-degitech/Mbyte/cmd/mbyte@latest   # needs Go 1.21+
# or
git clone https://github.com/Akira0412-degitech/Mbyte.git && cd Mbyte/go && go build -o mbyte ./cmd/mbyte
```

Dependencies: [cobra](https://github.com/spf13/cobra) (CLI) and
[minio/selfupdate](https://github.com/minio/selfupdate) (`mbyte update`).

---

## Usage

```
mbyte insert  --preset <name>|--hex <hex> <input> -o <output>
mbyte replace --preset <name>|--hex <hex> <input> -o <output>
mbyte check   <file>
mbyte update  [--check]
mbyte --list-presets
```

### Options (`insert` / `replace`)

| Option | Description |
| ------ | ----------- |
| `--preset <name>` | Use a named preset (see `--list-presets`) |
| `--hex <hex>` | Raw hex string, e.g. `89504e470d0a1a0a` |
| `-o`, `--output` | Output path (default `<input>.mbyte`, or the preset's extension) |
| `--force` | Allow overwriting the input file (off by default) |
| `--raw` | (`replace` only) Overwrite exactly `len(hex)` bytes, skipping boundary detection |
| `-y`, `--yes` | Skip the pre-write confirmation prompt |
| `--dry-run` | Show the preview and exit without writing |
| `--no-check` | Skip the post-write structural check |
| `-r`, `--recursive` | Recurse when the input is a directory (batch) |
| `--json` | Print batch results as JSON |
| `-v`, `--verbose` | Verbose logs |

Exactly one of `--preset` / `--hex` is required. The confirmation prompt appears only on an
interactive terminal; pipes, scripts, and batch runs proceed as if `-y` were given.

### Examples

Using the bundled sample `shell.php` (a harmless file with a web-shell string), from the repo root:

```bash
mbyte insert --preset png shell.php -o demo.png        # prepend a PNG header
mbyte insert --hex 89504e470d0a1a0a shell.php -o x.png # arbitrary signature
mbyte replace --preset jpeg demo.png -o demo.jpg       # replace the leading signature
mbyte replace --preset jpeg demo.png -o demo.jpg --dry-run   # preview only, no write
mbyte insert --preset png ./dir -o ./out -r --json     # batch a directory
```

A successful `insert` prints the detected format and a hexdump to stdout, and the structural-check
result to stderr:

```
[+] wrote demo.png
[+] detected format: PNG image data
[check] structure: FAIL (png: invalid format: Bad chunk length: 4294851584)
```

The `FAIL` is intentional: a prepended PNG signature does not make a valid PNG (see below).

### `replace` and `--raw`

By default `replace` detects where the file's existing signature ends and splices there, so it works
whether the new signature is shorter or longer — avoiding leftover bytes or data corruption. If the
current signature is unrecognized it errors out; pass `--raw` to force a fixed-length overwrite, or
use `insert` to just prepend.

---

## Structural check (`check`)

`check` reports both the signature guess and an **actual parse** using Go's standard-library
decoders, showing the difference between "signature matches" and "genuinely valid".

```bash
mbyte check shell.png.php
```

```
file: shell.png.php
[signature] PNG image data (matched 8 bytes)
[structure] FAIL (png: invalid format: Bad chunk length: 4294851584)
```

- Supported: PNG / JPEG / GIF, ZIP, ELF / PE. Valid → `PASS`, unsupported → `SKIP`, FAIL → exit `1`.
- `insert` / `replace` run this automatically after writing (disable with `--no-check`).
- They also compare the payload's SHA-256 before/after, showing `[MATCH]` / `[MISMATCH (shifted by
  ±N bytes)]` / `[UNVERIFIED]` — `insert` and boundary-detecting `replace` are always `[MATCH]`.

ZIP checks the End of Central Directory at the end of the file, so a prepended signature can never
make it valid — a clear example of why first-bytes-only spoofing fails a real parser.

---

## Self-update (`update`)

```bash
mbyte update --check    # report current vs. latest; change nothing
mbyte update            # download the latest and self-replace (prompts on a TTY)
mbyte update -y         # self-replace without the prompt
```

`update` fetches the latest GitHub release for your OS/arch, **verifies the archive against
`checksums.txt`, then re-verifies the extracted binary's SHA-256 before replacing** the running
executable. If any download or verification step fails, the existing binary is left untouched.

| Option | Description |
| ------ | ----------- |
| `--check` | Only report current vs. latest; download/replace nothing |
| `-y`, `--yes` | Skip the `[y/N]` confirmation prompt |

---

## Presets

| Name | Hex | Name | Hex |
| ---- | --- | ---- | --- |
| `png` | `89504e470d0a1a0a` | `elf` | `7f454c46` |
| `gif89a` | `474946383961` | `pe` | `4d5a` |
| `gif87a` | `474946383761` | `gzip` | `1f8b` |
| `jpeg` | `ffd8ffe0` | `7z` | `377abcaf271c` |
| `pdf` | `25504446` | `rar` | `52617221` |
| `zip` | `504b0304` | `javaclass` | `cafebabe` |

Run `mbyte --list-presets` to see them from the CLI. (BMP is detection-only, not an insert preset.)

---

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0` | Success |
| `1` | Runtime error (invalid hex, missing file, short replace, overwrite refused, unknown preset, batch failure, or a `check` structural FAIL) — prints `error: ...` to stderr |
| `2` | Argument error (e.g. `--preset` and `--hex` together, or an unknown flag) |

---

## Notes & limitations

- **Detection is a first-bytes guess**, not full validation — it may disagree with real `file(1)`.
  Use `mbyte check` for an actual parse (still not a complete check).
- **`insert` produces a file that is broken as a real format** (offsets shift); **`replace` discards
  the original leading bytes** (the input file itself is untouched).
- **Not a universal bypass**: Content-Type checks, re-encoding, and real parsing can't be fooled by
  magic bytes alone.
- **`replace` boundary detection only knows the presets above**; other formats require `--raw`.
  **JPEG detection matches the generic 3-byte `FF D8 FF`** (the preset still writes 4 bytes).
- **`checksums.txt` is not GPG-signed** — SHA-256 guards against corruption, not a compromised host
  or MITM forging both files. Verify through a trusted channel for high assurance.
- **Antivirus** may quarantine files containing web-shell strings; exclude your test folder or use
  harmless data.

---

## Development

```bash
cd go && go test ./... -v                 # Go (primary)
cd legacy-python && pip install -e ".[test]" && pytest -q   # Python reference
```

CI (GitHub Actions) builds and tests both. Shell completion is available via
`mbyte completion <bash|zsh|fish|powershell>`.

---

## Disclaimer

For **CTF practice** and **authorized** security testing only. Using it to bypass upload controls on
systems you do not own or have permission to test may be illegal. The author condones no misuse and
is not responsible for it. Use responsibly.

## License

MIT — see [LICENSE](LICENSE).
