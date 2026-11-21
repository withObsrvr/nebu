#!/usr/bin/env python3
"""Fail when a new package builds its own ledger stream.

Turning a ledger range into a stream of ledgers is one decision, and
`pkg/source` owns it: `pkg/source/internal/ledgerstream` holds the range
construction and the pump, and `pkg/source/rpc` and `pkg/source/storage` go
through it.

`cmd/nebu/fetch.go` does not. It constructs `ledgerbackend` streams and ranges
itself, and that second copy has already cost a real bug: when over-fetching
was fixed in `pkg/source/rpc`, `nebu fetch` kept asking for ten ledgers to
write one, so the fix looked complete while the command most people use still
had the old behaviour. Two copies of one decision is how that happens.

Collapsing fetch onto `pkg/source` is the real fix, but it moves a stable
surface (`docs/STABILITY.md`, `.api/source.txt`), so it is an owner's call
rather than something a passing agent should do. Until then this check freezes
the damage: the copies that exist are grandfathered by exact count, and a
third copy anywhere fails the build.

`pkg/source/internal/ledgerstream` is deliberately internal, so code outside
`pkg/source` cannot import it. That is why fetch re-implemented it, and why a
new caller will be tempted to do the same.
"""

from __future__ import annotations

import pathlib
import re
import sys

PATTERN = re.compile(
    r"ledgerbackend\.New[A-Za-z]*Stream"
    r"|ledgerbackend\.BoundedRange"
    r"|ledgerbackend\.UnboundedRange"
)

# Anything under this prefix owns the decision and may construct freely.
OWNER_PREFIX = "pkg/source/"

# Known copies outside the owner, frozen at the count that exists today.
# Lower these when fetch moves onto pkg/source; never raise them.
GRANDFATHERED = {"cmd/nebu/fetch.go": 4}

FIX = """
Build the ledger stream through pkg/source instead of constructing
ledgerbackend types directly:

  pkg/source/rpc.NewLedgerSource      — RPC-backed ranges
  pkg/source/storage.NewLedgerSource  — archive-backed ranges

Both route through pkg/source/internal/ledgerstream, which owns range
construction and the pump. If pkg/source does not expose what you need, extend
it there — do not start a third copy. See docs/STABILITY.md before changing
its public surface.
""".strip()


def main() -> int:
    root = pathlib.Path(__file__).resolve().parent.parent
    counts: dict[str, int] = {}

    for path in sorted(root.rglob("*.go")):
        rel = path.relative_to(root).as_posix()
        if rel.startswith(".direnv/") or rel.endswith("_test.go"):
            continue
        if rel.startswith(OWNER_PREFIX):
            continue
        try:
            hits = len(PATTERN.findall(path.read_text(encoding="utf-8")))
        except OSError as err:
            print(f"ERROR: could not read {rel}: {err}", file=sys.stderr)
            return 1
        if hits:
            counts[rel] = hits

    failures = []
    for rel, hits in sorted(counts.items()):
        allowed = GRANDFATHERED.get(rel, 0)
        if hits > allowed:
            failures.append(
                f"  {rel}: {hits} direct ledgerbackend construction(s), "
                f"{allowed} grandfathered"
            )

    # A grandfathered entry that has dropped to zero, or a file that no longer
    # exists, means the copy is gone — stop grandfathering it so it cannot
    # come back for free.
    for rel, allowed in sorted(GRANDFATHERED.items()):
        hits = counts.get(rel, 0)
        if hits < allowed:
            failures.append(
                f"  {rel}: now {hits} construction(s), grandfathered for "
                f"{allowed}. Lower GRANDFATHERED in {pathlib.Path(__file__).name} "
                f"to {hits} so the copies cannot return."
            )

    if failures:
        print("ERROR: ledger-stream construction outside pkg/source has changed.")
        print()
        print("\n".join(failures))
        print()
        print(FIX)
        return 1

    total = sum(counts.values())
    print(
        f"✓ Ledger-stream construction owned by pkg/source "
        f"({total} grandfathered copy/copies outside it, unchanged)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
