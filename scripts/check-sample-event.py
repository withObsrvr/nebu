#!/usr/bin/env python3
"""Verify the SAMPLE_EVENT block on the landing page matches real output.

The documented sample drifted from reality once already: it advertised four
fewer fields than token-transfer actually emits, including the
transactionIndex/operationIndex pair a reader needs in order to believe nebu
gives them a resumable cursor. docs-smoke verified the CLI *commands* but
never the sample JSON, so nothing caught it.

Reads a token-transfer event from stdin and compares its field paths against
the sample in docs/index.html. Fails in both directions: a documented field
that does not exist is a lie, and an emitted field that is not documented is
an undersell.

Fields in PENDING_REBUILD are allowed to be documented but absent, because
the registry processors are built against a released nebu and only start
emitting newly added envelope fields once they are rebuilt against it.
"""

import html
import json
import re
import sys
from pathlib import Path

# Envelope fields this tree adds that the installed registry processors do not
# emit yet. Remove an entry once the registry has been rebuilt against the
# nebu release that introduced it.
PENDING_REBUILD = {"network"}

ROOT = Path(__file__).resolve().parent.parent


def field_paths(obj, prefix=""):
    if isinstance(obj, dict):
        for key, value in obj.items():
            yield from field_paths(value, f"{prefix}.{key}" if prefix else key)
    else:
        yield prefix


def documented_sample():
    page = (ROOT / "docs" / "index.html").read_text()
    index = page.find("SAMPLE_EVENT")
    if index == -1:
        sys.exit("docs/index.html: no SAMPLE_EVENT section found")
    match = re.search(r"<(pre|code)[^>]*>(.*?)</\1>", page[index:index + 4000], re.S)
    if not match:
        sys.exit("docs/index.html: no code block after SAMPLE_EVENT")
    text = html.unescape(re.sub(r"<[^>]+>", "", match.group(2)))
    body = text.split("// teal")[0].strip()
    try:
        return json.loads(body)
    except json.JSONDecodeError as err:
        sys.exit(f"documented SAMPLE_EVENT is not valid JSON: {err}\n{body}")


def main():
    raw = sys.stdin.readline()
    if not raw.strip():
        sys.exit("no event on stdin")
    actual = json.loads(raw)

    if "transfer" not in actual:
        sys.exit(
            f"expected a transfer event, got payload key(s): "
            f"{sorted(k for k in actual if not k.startswith('_') and k != 'meta')}"
        )

    documented = set(field_paths(documented_sample()))
    emitted = set(field_paths(actual))

    missing = sorted(f for f in documented - emitted if f not in PENDING_REBUILD)
    undocumented = sorted(emitted - documented)

    if missing:
        print("SAMPLE_EVENT documents fields that are not emitted:", file=sys.stderr)
        for field in missing:
            print(f"  - {field}", file=sys.stderr)
    if undocumented:
        print("token-transfer emits fields the SAMPLE_EVENT omits:", file=sys.stderr)
        for field in undocumented:
            print(f"  + {field}", file=sys.stderr)

    if missing or undocumented:
        print(
            "\nUpdate the SAMPLE_EVENT block in docs/index.html so the shop "
            "window matches the product.",
            file=sys.stderr,
        )
        return 1

    pending = sorted(documented & PENDING_REBUILD)
    note = f" ({', '.join(pending)} pending registry rebuild)" if pending else ""
    print(f"SAMPLE_EVENT matches token-transfer output{note}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
