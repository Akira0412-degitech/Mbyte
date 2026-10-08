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


def replace_magic_bytes(input_path: str, output_path: str, hex_bytes: str) -> bytes:
    """Overwrite the first ``len(hex_bytes)`` bytes with ``hex_bytes``.

    Remaining bytes are preserved. Raises ``MbyteError`` if the input file is
    shorter than the replacement bytes. Returns the new leading bytes.
    """
    magic = parse_hex(hex_bytes)
    original = _read_input(input_path)
    if len(original) < len(magic):
        raise MbyteError(
            f"input file is too short ({len(original)} bytes) to replace "
            f"{len(magic)} bytes; use 'insert' instead"
        )
    with open(output_path, "wb") as fh:
        fh.write(magic + original[len(magic):])
    return magic


def detect_format(data: bytes) -> str:
    """Guess a file(1)-style format name from the leading bytes."""
    head = data[:16].hex()
    for sig, name in _SIGNATURES:
        if head.startswith(sig):
            return name
    return "data (unknown / unrecognized signature)"


def hexdump(data: bytes, length: int = 16) -> str:
    """Return an ``xxd``-style dump of up to ``length`` bytes of ``data``."""
    chunk = data[:length]
    hex_part = " ".join(f"{b:02x}" for b in chunk)
    # pad hex column so the ascii gutter lines up (3 chars per byte)
    hex_part = hex_part.ljust(length * 3 - 1)
    ascii_part = "".join(chr(b) if 32 <= b < 127 else "." for b in chunk)
    return f"00000000: {hex_part}  {ascii_part}"
