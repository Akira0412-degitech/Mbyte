"""Magic-byte signature presets.

Keys are preset names; values are lowercase hex strings (no spaces).
"""

PRESETS = {
    "png": "89504e470d0a1a0a",
    "gif89a": "474946383961",
    "gif87a": "474946383761",
    "jpeg": "ffd8ffe0",
    "pdf": "25504446",
    "zip": "504b0304",
}


def list_presets() -> str:
    """Return a human-readable table of available presets."""
    width = max(len(name) for name in PRESETS)
    lines = ["Available presets:", ""]
    lines.append(f"  {'NAME'.ljust(width)}  HEX")
    lines.append(f"  {'-' * width}  {'-' * 16}")
    for name, hex_bytes in PRESETS.items():
        lines.append(f"  {name.ljust(width)}  {hex_bytes}")
    return "\n".join(lines)
