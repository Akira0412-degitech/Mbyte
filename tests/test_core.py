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
