"""Command-line interface for mbyte."""

from __future__ import annotations

import argparse
import os
import sys

from . import __version__
from .core import (
    MbyteError,
    detect_format,
    hexdump,
    insert_magic_bytes,
    replace_magic_bytes,
)
from .presets import PRESETS, list_presets


def _add_common_args(sub: argparse.ArgumentParser) -> None:
    sub.add_argument("input_file", help="path to the input file")
    sub.add_argument(
        "-o", "--output", help="output file path (default: <input_file>.mbyte)"
    )
    group = sub.add_mutually_exclusive_group(required=True)
    group.add_argument("--preset", help="named magic-byte preset (see --list-presets)")
    group.add_argument("--hex", dest="hex_bytes", help="raw hex string, e.g. 89504e47")
    sub.add_argument(
        "--force",
        action="store_true",
        help="allow writing over the input file (disabled by default)",
    )
    sub.add_argument(
        "-v", "--verbose", action="store_true", help="print detailed logs"
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="mbyte",
        description=(
            "Insert or replace a file's leading magic bytes. "
            "For CTF / authorized-testing education only."
        ),
    )
    parser.add_argument("--version", action="version", version=f"mbyte {__version__}")
    parser.add_argument(
        "--list-presets",
        action="store_true",
        help="list available magic-byte presets and exit",
    )

    subparsers = parser.add_subparsers(dest="command")
    insert_p = subparsers.add_parser(
        "insert", help="prepend magic bytes (keeps original content)"
    )
    _add_common_args(insert_p)
    replace_p = subparsers.add_parser(
        "replace", help="overwrite the first N bytes with magic bytes"
    )
    _add_common_args(replace_p)
    return parser


def _resolve_hex(args: argparse.Namespace) -> str:
    if args.preset is not None:
        if args.preset not in PRESETS:
            valid = ", ".join(PRESETS)
            raise MbyteError(
                f"unknown preset {args.preset!r}; valid presets: {valid}"
            )
        return PRESETS[args.preset]
    return args.hex_bytes


def _resolve_output(args: argparse.Namespace) -> str:
    if args.output:
        output = args.output
    else:
        output = f"{args.input_file}.mbyte"
    if os.path.abspath(output) == os.path.abspath(args.input_file) and not args.force:
        raise MbyteError(
            "refusing to overwrite the input file; choose a different -o/--output "
            "path or pass --force"
        )
    return output


def run(args: argparse.Namespace) -> int:
    hex_bytes = _resolve_hex(args)
    output = _resolve_output(args)

    if args.verbose:
        print(f"[*] command    : {args.command}")
        print(f"[*] input      : {args.input_file}")
        print(f"[*] output     : {output}")
        print(f"[*] magic hex  : {hex_bytes}")

    if args.command == "insert":
        insert_magic_bytes(args.input_file, output, hex_bytes)
    else:
        replace_magic_bytes(args.input_file, output, hex_bytes)

    with open(output, "rb") as fh:
        head = fh.read(16)

    print(f"[+] wrote {output}")
    print(f"[+] detected format: {detect_format(head)}")
    print(hexdump(head))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.list_presets:
        print(list_presets())
        return 0

    if args.command is None:
        parser.print_help()
        return 1

    try:
        return run(args)
    except FileNotFoundError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except MbyteError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
