"""Core insert/replace logic and helpers for mbyte.

Functions here RAISE exceptions on bad input (``MbyteError`` or the builtin
``FileNotFoundError``). The CLI layer is responsible for turning those into
friendly messages and exit codes. This keeps the core easy to unit-test.
"""

from __future__ import annotations

import os
import string


class MbyteError(Exception):
    """Raised for invalid user input or impossible operations."""


_HEX_DIGITS = set(string.hexdigits.lower())

# Signature -> human-readable format name (file(1)-ish), longest match wins.
_SIGNATURES = [
    ("89504e470d0a1a0a", "PNG image data"),
    ("474946383961", "GIF image data, version 89a"),
    ("474946383761", "GIF image data, version 87a"),
    # JPEG is matched on the generic 3-byte FF D8 FF prefix so that both JFIF
    # (ffd8ffe0) and EXIF (ffd8ffe1) variants are recognized. This means the
    # detected JPEG boundary is intentionally 3 bytes -- one byte short of the
    # 4-byte APP0/APP1 marker that PRESETS["jpeg"] writes -- so a `replace` onto
    # a JPEG may leave that 4th, marker-ambiguous byte behind. Documented
    # limitation, not a bug: we only ever claim the 3 bytes we actually know.
    ("ffd8ff", "JPEG image data"),
    ("25504446", "PDF document"),
    ("504b0304", "Zip archive data"),
    ("424d", "PC bitmap (BMP) image data"),
    ("7f454c46", "ELF executable"),
    ("4d5a", "PE32 executable (MS-DOS MZ)"),
    ("1f8b", "gzip compressed data"),
    ("377abcaf271c", "7-zip archive data"),
    ("52617221", "RAR archive data"),
    ("cafebabe", "Java class data"),
]


def parse_hex(hex_bytes: str) -> bytes:
    """Validate and convert a hex string to bytes.

    Raises ``MbyteError`` for odd length or non-hex characters.
    """
    cleaned = hex_bytes.strip().lower().replace(" ", "")
    if cleaned == "":
        raise MbyteError("hex string is empty")
    if len(cleaned) % 2 != 0:
        raise MbyteError(
            f"hex string has an odd number of digits ({len(cleaned)}): {cleaned!r}"
        )
    bad = sorted({c for c in cleaned if c not in _HEX_DIGITS})
    if bad:
        raise MbyteError(
            f"hex string contains invalid characters {bad}: {cleaned!r}"
        )
    return bytes.fromhex(cleaned)


def _read_input(input_path: str) -> bytes:
    if not os.path.exists(input_path):
        raise FileNotFoundError(f"input file not found: {input_path}")
    if os.path.isdir(input_path):
        raise MbyteError(f"input path is a directory, not a file: {input_path}")
    with open(input_path, "rb") as fh:
        return fh.read()


def insert_magic_bytes(input_path: str, output_path: str, hex_bytes: str) -> bytes:
    """Prepend ``hex_bytes`` to the input file, writing the result to output.

    The input file is left unchanged. Returns the leading bytes of the output
    (the magic bytes) for preview/detection.
    """
    magic = parse_hex(hex_bytes)
    original = _read_input(input_path)
    with open(output_path, "wb") as fh:
        fh.write(magic + original)
    return magic


def replace_magic_bytes(
    input_path: str,
    output_path: str,
    hex_bytes: str,
    *,
    raw: bool = False,
) -> bytes:
    """Replace the input file's leading signature with ``hex_bytes``.

    By default (``raw=False``) the length of the file's EXISTING signature is
    detected via :func:`detect_signature_length` and the splice happens at that
    boundary: ``output = magic + original[boundary:]``. This is correct whether
    the new signature is shorter, longer, or the same length as the old one,
    because the cut point is where the old signature actually ends -- not how
    long the new one happens to be. If the current format isn't recognized,
    ``MbyteError`` is raised (pass ``raw=True`` to force a fixed-length
    overwrite, or use ``insert`` to just prepend bytes).

    With ``raw=True`` the old, fixed-length behavior is used: overwrite exactly
    ``len(hex_bytes)`` bytes from the start and keep the rest. This may corrupt
    data or leave garbage bytes if the length doesn't match the file's real
    signature; it raises ``MbyteError`` if the input file is shorter than the
    replacement bytes.

    Returns the new leading bytes.
    """
    magic = parse_hex(hex_bytes)
    original = _read_input(input_path)

    if raw:
        if len(original) < len(magic):
            raise MbyteError(
                f"input file is too short ({len(original)} bytes) to replace "
                f"{len(magic)} bytes; use 'insert' instead"
            )
        output = magic + original[len(magic):]
    else:
        boundary = detect_signature_length(original)
        if boundary is None:
            raise MbyteError(
                "could not recognize the current file's signature, so there is "
                "no safe splice point for 'replace'; pass raw=True (CLI: --raw) "
                "to force a fixed-length overwrite, or use 'insert' to just "
                "prepend bytes"
            )
        output = magic + original[boundary:]

    with open(output_path, "wb") as fh:
        fh.write(output)
    return magic


def _match_signature(data: bytes) -> tuple[str, str] | None:
    """Return the ``(hex_sig, name)`` of the LONGEST ``_SIGNATURES`` entry that
    is a prefix of ``data``, or ``None`` if nothing matches.

    Longest-match-wins avoids a short generic prefix shadowing a more specific
    signature, and gives :func:`detect_format` and
    :func:`detect_signature_length` a single source of truth so they can't
    disagree about what matched.
    """
    head = data[:16].hex()
    best = None
    for sig, name in _SIGNATURES:
        if head.startswith(sig) and (best is None or len(sig) > len(best[0])):
            best = (sig, name)
    return best


def detect_format(data: bytes) -> str:
    """Guess a file(1)-style format name from the leading bytes."""
    match = _match_signature(data)
    if match is None:
        return "data (unknown / unrecognized signature)"
    return match[1]


def detect_signature_length(data: bytes) -> int | None:
    """Return the byte-length of the longest known signature that matches the
    start of ``data``, or ``None`` if nothing in ``_SIGNATURES`` matches."""
    match = _match_signature(data)
    if match is None:
        return None
    return len(match[0]) // 2


def hexdump(data: bytes, length: int = 16) -> str:
    """Return an ``xxd``-style dump of up to ``length`` bytes of ``data``."""
    chunk = data[:length]
    hex_part = " ".join(f"{b:02x}" for b in chunk)
    # pad hex column so the ascii gutter lines up (3 chars per byte)
    hex_part = hex_part.ljust(length * 3 - 1)
    ascii_part = "".join(chr(b) if 32 <= b < 127 else "." for b in chunk)
    return f"00000000: {hex_part}  {ascii_part}"
