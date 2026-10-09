"""Unit tests for mbyte.core and presets."""

import pytest

from mbyte.core import (
    MbyteError,
    detect_format,
    detect_signature_length,
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


def test_replace_raw_overwrites_head_keeps_tail(tmp_path):
    # b"ABCDEFGH" has no recognized signature, so the fixed-length overwrite
    # path (raw=True) is the right fit for this behavior.
    src = _write(tmp_path / "in.bin", b"ABCDEFGH")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, "00112233", raw=True)  # 4 bytes

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
    # raw=True exercises the fixed-length too-short guard directly
    with pytest.raises(MbyteError):
        replace_magic_bytes(src, out, "89504e470d0a1a0a", raw=True)  # 8 bytes


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


def test_replace_with_shorter_magic_splices_at_detected_boundary(tmp_path):
    """New signature shorter than the old one: detection (default) splices at
    the OLD signature's real end, so no leftover old-signature bytes survive.
    """
    # PNG signature is 8 bytes; GIF89a is only 6. Boundary detected = 8 (PNG).
    src = _write(tmp_path / "in.bin", bytes.fromhex(PRESETS["png"]) + b"REST")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, PRESETS["gif89a"])

    result = (tmp_path / "out.bin").read_bytes()
    # exactly the new GIF signature followed by the untouched original tail --
    # none of the old 8-byte PNG signature leaks through
    assert result == bytes.fromhex(PRESETS["gif89a"]) + b"REST"
    assert detect_format(result) == "GIF image data, version 89a"


def test_replace_with_longer_magic_preserves_following_data(tmp_path):
    """Inverse case: new signature longer than the old one. Detection splices
    at the old 6-byte boundary, so the full original tail survives untouched --
    the longer new signature does NOT eat into the real data.
    """
    # GIF89a signature is 6 bytes; PNG is 8. Boundary detected = 6 (GIF).
    src = _write(tmp_path / "in.bin", bytes.fromhex(PRESETS["gif89a"]) + b"REST")
    out = str(tmp_path / "out.bin")
    replace_magic_bytes(src, out, PRESETS["png"])

    result = (tmp_path / "out.bin").read_bytes()
    assert result == bytes.fromhex(PRESETS["png"]) + b"REST"
    assert detect_format(result) == "PNG image data"


def test_replace_raw_reproduces_old_fixed_length_behavior(tmp_path):
    """raw=True is the explicit escape hatch: overwrite exactly len(new_hex)
    bytes from the start, reproducing the old (lossy) behavior on purpose.
    """
    # shorter new signature: leftover old PNG bytes survive after the new GIF
    src = _write(tmp_path / "s.bin", bytes.fromhex(PRESETS["png"]) + b"REST")
    out = str(tmp_path / "s.out")
    replace_magic_bytes(src, out, PRESETS["gif89a"], raw=True)
    result = (tmp_path / "s.out").read_bytes()
    assert result == (
        bytes.fromhex(PRESETS["gif89a"]) + bytes.fromhex(PRESETS["png"])[6:] + b"REST"
    )

    # longer new signature: the first 2 bytes of the tail get eaten
    src = _write(tmp_path / "l.bin", bytes.fromhex(PRESETS["gif89a"]) + b"REST")
    out = str(tmp_path / "l.out")
    replace_magic_bytes(src, out, PRESETS["png"], raw=True)
    result = (tmp_path / "l.out").read_bytes()
    assert result == bytes.fromhex(PRESETS["png"]) + b"ST"


def test_replace_unrecognized_format_requires_raw(tmp_path):
    """Without raw, an unrecognized current signature has no safe splice point
    and raises; with raw=True it falls back to the old fixed-length overwrite.
    """
    src = _write(tmp_path / "in.bin", b"NOTAREALSIG")
    out = str(tmp_path / "out.bin")

    with pytest.raises(MbyteError):
        replace_magic_bytes(src, out, PRESETS["png"])

    # raw=True forces the fixed-length overwrite and succeeds
    replace_magic_bytes(src, out, PRESETS["png"], raw=True)
    result = (tmp_path / "out.bin").read_bytes()
    magic = bytes.fromhex(PRESETS["png"])
    assert result == magic + b"NOTAREALSIG"[len(magic):]


def test_detect_signature_length(tmp_path):
    assert detect_signature_length(bytes.fromhex(PRESETS["png"]) + b"REST") == 8
    assert detect_signature_length(bytes.fromhex(PRESETS["gif89a"]) + b"REST") == 6
    # jpeg detection matches the generic 3-byte FF D8 FF prefix (Step 2),
    # one byte short of the 4-byte ffd8ffe0 preset marker
    assert detect_signature_length(bytes.fromhex(PRESETS["jpeg"]) + b"REST") == 3
    assert detect_signature_length(b"NOTAREALSIG") is None
