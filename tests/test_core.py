"""Unit tests for mbyte.core and presets."""

import pytest

from mbyte.core import (
    MbyteError,
    detect_format,
    insert_magic_bytes,
    parse_hex,
    replace_magic_bytes,
)
from mbyte.presets import PRESETS


def _write(path, data: bytes):
    path.write_bytes(data)
    return str(path)


def test_insert_prepends_and_preserves(tmp_path):
    src = _write(tmp_path / "in.bin", b"hello")
    out = str(tmp_path / "out.bin")
    insert_magic_bytes(src, out, "89504e47")

    result = (tmp_path / "out.bin").read_bytes()
    assert result == bytes.fromhex("89504e47") + b"hello"
    # input untouched
    assert (tmp_path / "in.bin").read_bytes() == b"hello"


def test_replace_overwrites_head_keeps_tail(tmp_path):
    src = _write(tmp_path / "in.bin", b"ABCDEFGH")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, "00112233")  # 4 bytes

    result = (tmp_path / "out.bin").read_bytes()
    assert result == bytes.fromhex("00112233") + b"EFGH"


@pytest.mark.parametrize("bad", ["abc", "zz", "89504e4g", "12 3"])
def test_parse_hex_rejects_invalid(bad):
    with pytest.raises(MbyteError):
        parse_hex(bad)


def test_invalid_hex_in_insert_raises(tmp_path):
    src = _write(tmp_path / "in.bin", b"data")
    out = str(tmp_path / "out.bin")
    with pytest.raises(MbyteError):
        insert_magic_bytes(src, out, "xyz")


def test_replace_too_short_raises(tmp_path):
    src = _write(tmp_path / "in.bin", b"ab")  # 2 bytes
    out = str(tmp_path / "out.bin")
    with pytest.raises(MbyteError):
        replace_magic_bytes(src, out, "89504e470d0a1a0a")  # 8 bytes


def test_missing_input_raises(tmp_path):
    out = str(tmp_path / "out.bin")
    with pytest.raises(FileNotFoundError):
        insert_magic_bytes(str(tmp_path / "nope.bin"), out, "89504e47")


def test_preset_values_are_known():
    assert PRESETS["png"] == "89504e470d0a1a0a"
    assert PRESETS["jpeg"] == "ffd8ffe0"
    assert PRESETS["pdf"] == "25504446"
    assert PRESETS["zip"] == "504b0304"


def test_detect_format_png(tmp_path):
    src = _write(tmp_path / "in.bin", b"hello")
    out = str(tmp_path / "out.png")
    insert_magic_bytes(src, out, PRESETS["png"])
    assert detect_format((tmp_path / "out.png").read_bytes()) == "PNG image data"


def test_replace_with_shorter_magic_keeps_original_tail_bytes(tmp_path):
    """replace only overwrites len(hex) bytes; if the new signature is
    shorter than the one it's replacing, the leftover bytes of the OLD
    signature are left in place, right after the new one.
    """
    # PNG signature is 8 bytes; GIF89a is only 6.
    src = _write(tmp_path / "in.bin", bytes.fromhex(PRESETS["png"]) + b"REST")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, PRESETS["gif89a"])

    result = (tmp_path / "out.bin").read_bytes()
    # first 6 bytes are the new GIF signature
    assert result[:6] == bytes.fromhex(PRESETS["gif89a"])
    # the last 2 bytes of the ORIGINAL PNG signature (1a 0a) survive untouched,
    # followed by the original tail data
    assert result[6:] == bytes.fromhex(PRESETS["png"])[6:] + b"REST"
    # detect_format only checks the leading bytes against known signatures,
    # so this still reads as a valid GIF even though PNG bytes trail it
    assert detect_format(result) == "GIF image data, version 89a"


def test_replace_with_longer_magic_eats_into_following_data(tmp_path):
    """The inverse case: new signature longer than the one it replaces.

    replace_magic_bytes always overwrites exactly len(new_hex) bytes from
    the start, so if the new signature is LONGER than the old one, it also
    overwrites some of the bytes that originally followed the old
    signature -- not just the signature itself.
    """
    # GIF89a signature is 6 bytes; PNG is 8, i.e. 2 bytes longer.
    src = _write(tmp_path / "in.bin", bytes.fromhex(PRESETS["gif89a"]) + b"REST")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, PRESETS["png"])

    result = (tmp_path / "out.bin").read_bytes()
    # the first 2 bytes of "REST" (b"RE") got consumed by the longer PNG
    # signature, so only b"ST" of the original tail survives
    assert result == bytes.fromhex(PRESETS["png"]) + b"ST"
    assert detect_format(result) == "PNG image data"
