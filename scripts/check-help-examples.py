#!/usr/bin/env python3
"""Verify the flags in nebu's root help exist on the processors it names.

The root help is the first thing anyone reads, and nothing checked it. By
v0.7.2 two of its four quickstart commands were broken: it advertised
`jq 'select(.type == "transfer")'` against events that have no `.type` field —
0 results where the correct filter returns hundreds — and
`token-transfer --start N --end M`, which exits with "unknown flag: --start"
because the processor exposes --start-ledger/--end-ledger.

Both were plain wrong rather than subtly stale, and both survived because
docs-smoke verified the README and the cookbook while the front door was
never looked at.

Usage: check-help-examples.py <nebu-binary> <processor-binary>...

Reads each processor's real flag set from its --help and fails if the root
help uses a flag the processor does not have.
"""

import re
import subprocess
import sys
from pathlib import Path

FLAG = re.compile(r"--[a-z][a-z0-9-]*")


def flags_of(binary):
    """Every long flag the binary's help advertises."""
    out = subprocess.run([binary, "--help"], capture_output=True, text=True).stdout
    return set(FLAG.findall(out))


def stages(help_text):
    """Each pipeline stage in the help, paired with the line it came from.

    Flags have to be attributed per stage rather than per line.
    `nebu fetch --last 100 | token-transfer` puts --last and token-transfer on
    one line, but the flag belongs to fetch; checking it against the processor
    reports a failure that is not there.
    """
    for line in help_text.splitlines():
        text = line.strip()
        if text.startswith("#"):
            continue
        for stage in text.rstrip("\\").split("|"):
            if stage.strip():
                yield stage.strip(), line


def command_of(stage):
    """The command a stage invokes, by basename, or "" if it has no words."""
    words = stage.split()
    return Path(words[0]).name if words else ""


def main():
    if len(sys.argv) < 3:
        sys.exit("usage: check-help-examples.py <nebu> <processor>...")

    nebu, processors = sys.argv[1], sys.argv[2:]
    help_text = subprocess.run([nebu, "--help"], capture_output=True, text=True).stdout
    if not help_text.strip():
        sys.exit("nebu --help produced nothing")

    known = {Path(p).name: flags_of(p) for p in processors}

    problems = []
    checked = 0
    for stage, line in stages(help_text):
        name = command_of(stage)
        if name not in known:
            continue
        checked += 1
        for flag in FLAG.findall(stage):
            if flag not in known[name]:
                problems.append(
                    f"  {name}: root help uses {flag}, which it does not accept"
                    f"\n      in: {line.strip()}"
                )

    if problems:
        print("nebu --help advertises flags that do not exist:", file=sys.stderr)
        for problem in sorted(set(problems)):
            print(problem, file=sys.stderr)
        print(
            "\nThe root help is the first thing a reader runs. Fix the examples "
            "in cmd/nebu/main.go.",
            file=sys.stderr,
        )
        return 1

    print(f"root help: {checked} pipeline stages check out against {len(known)} processors")
    return 0


if __name__ == "__main__":
    sys.exit(main())
