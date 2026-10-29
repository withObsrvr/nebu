#!/usr/bin/env python3
"""Verify the SAMPLE_EVENT block on the landing page matches real output.

The documented sample drifted from reality once already: it advertised four
fewer fields than token-transfer actually emits, including the
transactionIndex/operationIndex pair a reader needs in order to believe nebu
gives them a resumable cursor. docs-smoke verified the CLI *commands* but
never the sample JSON, so nothing caught it.

Reads a token-transfer event from stdin and compares the sample in
docs/index.html against it on three axes:

  names  Fails in both directions. A documented field that does not exist is
         a lie; an emitted field that is not documented is an undersell.
  types  A documented field whose JSON type differs from the real one is the
         more dangerous error, because someone writing a consumer against the
         docs gets it wrong and only finds out at runtime. "closedAtUnix" is
         the live example: it is a *string* of digits, and documenting it as a
         number would be an easy and invisible mistake.
  values Checked only where the value is contractual rather than
         illustrative: _schema is a schema identifier and must match exactly,
         and _nebu_version must look like a real version. Everything else is
         free to be an abbreviated example.

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
#
# Empty as of the registry's Protocol 30 upgrade: the processors now build
# against nebu v0.6.17, so "network" is emitted for real and is enforced like
# any other field rather than excused.
PENDING_REBUILD: set[str] = set()

ROOT = Path(__file__).resolve().parent.parent


VERSION_RE = re.compile(r"^v\d+\.\d+\.\d+(?:-[\w.]+)?$|^dev$")

# Shape checks for fields that cannot be compared against real output because
# the installed processors do not emit them yet.
PENDING_SHAPE = {
    "network": lambda v: isinstance(v, str) and v != "",
}


def field_paths(obj, prefix=""):
    yield from (path for path, _ in leaves(obj, prefix))


def leaves(obj, prefix=""):
    """Yield (dotted path, value) for every leaf in obj."""
    if isinstance(obj, dict):
        for key, value in obj.items():
            yield from leaves(value, f"{prefix}.{key}" if prefix else key)
    else:
        yield prefix, obj


def json_type(value):
    if isinstance(value, bool):
        return "boolean"
    if isinstance(value, (int, float)):
        return "number"
    if isinstance(value, str):
        return "string"
    if value is None:
        return "null"
    return type(value).__name__


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

    sample = documented_sample()
    documented_leaves = dict(leaves(sample))
    emitted_leaves = dict(leaves(actual))
    documented = set(documented_leaves)
    emitted = set(emitted_leaves)

    missing = sorted(f for f in documented - emitted if f not in PENDING_REBUILD)
    undocumented = sorted(emitted - documented)

    wrong_type = [
        (path, json_type(documented_leaves[path]), json_type(emitted_leaves[path]))
        for path in sorted(documented & emitted)
        if json_type(documented_leaves[path]) != json_type(emitted_leaves[path])
    ]

    wrong_value = []
    if "_schema" in documented & emitted and documented_leaves["_schema"] != emitted_leaves["_schema"]:
        wrong_value.append(
            f'_schema is a contract identifier and must match exactly: '
            f'documented {documented_leaves["_schema"]!r}, emitted {emitted_leaves["_schema"]!r}'
        )
    version_value = documented_leaves.get("_nebu_version")
    if version_value is not None and not VERSION_RE.match(str(version_value)):
        wrong_value.append(
            f'_nebu_version {version_value!r} is not version-shaped; use a real '
            f'released version, not a guess at the next one'
        )
    for path, ok in PENDING_SHAPE.items():
        if path in documented_leaves and not ok(documented_leaves[path]):
            wrong_value.append(f"{path} has an implausible documented value: {documented_leaves[path]!r}")

    if missing:
        print("SAMPLE_EVENT documents fields that are not emitted:", file=sys.stderr)
        for field in missing:
            print(f"  - {field}", file=sys.stderr)
    if undocumented:
        print("token-transfer emits fields the SAMPLE_EVENT omits:", file=sys.stderr)
        for field in undocumented:
            print(f"  + {field}", file=sys.stderr)

    if wrong_type:
        print("SAMPLE_EVENT documents the wrong JSON type:", file=sys.stderr)
        for path, documented_type, actual_type in wrong_type:
            print(f"  ~ {path}: documented as {documented_type}, emitted as {actual_type}", file=sys.stderr)
    for problem in wrong_value:
        print(f"  ! {problem}", file=sys.stderr)

    if missing or undocumented or wrong_type or wrong_value:
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
