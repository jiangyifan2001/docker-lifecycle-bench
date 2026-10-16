from __future__ import annotations

import argparse
import sys

from .client import DlbenchClient


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(
        prog="dlbench-client",
        description="Thin wrapper around the canonical dlbench Go binary",
    )
    result.add_argument("--binary", default="dlbench")
    subcommands = result.add_subparsers(dest="command", required=True)

    run = subcommands.add_parser("run")
    run.add_argument("--spec", required=True)

    validate = subcommands.add_parser("validate")
    validate.add_argument("--spec", required=True)
    return result


def main() -> int:
    args = parser().parse_args()
    client = DlbenchClient(args.binary)
    if args.command == "run":
        return client.run_campaign(args.spec).returncode
    if args.command == "validate":
        return client.validate(args.spec).returncode
    raise AssertionError(f"unhandled command: {args.command}")


if __name__ == "__main__":
    sys.exit(main())

