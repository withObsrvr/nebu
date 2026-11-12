#!/usr/bin/env python3
"""Verify every documented token-transfer sample matches real output.

The landing page's sample drifted from reality once already: it advertised
four fewer fields than token-transfer actually emits, including the
transactionIndex/operationIndex pair a reader needs in order to believe nebu
gives them a resumable cursor. docs-smoke verified the CLI *commands* but
never the sample JSON, so nothing caught it.

Checking only the landing page then let the same drift happen everywhere else.
By the v0.7.2 release the README and docs/PIPELINE.md were each missing
network, meta.operationIndex and meta.inSuccessfulTx, and docs/quickstart.html
— the page a new reader actually lands on — was missing seven fields
including the whole resumable-cursor pair. The landing page was the only
sample that was right, because it was the only one being checked. So this
checks all of them.

Samples come in two kinds, declared in SAMPLES:

  complete   The sample is a claim about the full envelope, so a field the
             product emits and the sample omits is an undersell and fails.
             The landing page is this: it is the shop window.
  excerpt    The sample is deliberately trimmed for brevity, so omissions are
             fine — but every field it *does* show must exist and be correctly
             typed. An excerpt may be incomplete; it may not be wrong.

Reads a token-transfer event from stdin and compares each sample against it on
three axes:

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


# Every place a token-transfer event is documented, and whether the sample
# claims to be the whole envelope. Add a row when a new page documents one;
# the point of this list is that forgetting to is what caused the drift.
SAMPLES = [
    ("docs/index.html", "complete"),
    # Was an elided sketch with a literal "..." inside the JSON, so it could
    # never be parsed or checked — and it hid the transactionIndex/operationIndex
    # pair, on the page a new reader lands on first.
    ("docs/quickstart.html", "complete"),
    ("README.md", "complete"),
    ("docs/PIPELINE.md", "complete"),
]

SCHEMA_ID = "nebu.token_transfer.v1"


def extract_sample(relative_path):
    """Return the documented token-transfer event in relative_path.

    Handles both HTML (tags stripped, entities unescaped) and Markdown fenced
    blocks, because the samples live in both and a reader does not care which.
    """
    path = ROOT / relative_path
    if not path.exists():
        sys.exit(f"{relative_path}: no such file")
    page = path.read_text()

    if path.suffix == ".html":
        blocks = [
            html.unescape(re.sub(r"<[^>]+>", "", match.group(2)))
            for match in re.finditer(r"<(pre|code)[^>]*>(.*?)</\1>", page, re.S)
        ]
    else:
        # Walk the fences rather than regexing them: a lazy pattern happily
        # pairs one block's closing fence with another's opening one and
        # returns the prose in between.
        blocks, current = [], None
        for line in page.splitlines():
            if line.startswith("```"):
                if current is None:
                    current = []
                else:
                    blocks.append("\n".join(current))
                    current = None
                continue
            if current is not None:
                current.append(line)

    for block in blocks:
        if SCHEMA_ID not in block:
            continue
        # Trailing annotations after the JSON (the landing page labels colours
        # in a comment) are not part of the document.
        body = block.split("// teal")[0].strip()
        try:
            return json.loads(body)
        except json.JSONDecodeError as err:
            sys.exit(f"{relative_path}: documented sample is not valid JSON: {err}\n{body}")

    sys.exit(f"{relative_path}: no code block containing {SCHEMA_ID}")


def check_sample(relative_path, kind, actual, emitted_leaves):
    """Compare one documented sample against real output.

    Returns a list of problem lines; empty means the sample is honest.
    """
    sample = extract_sample(relative_path)
    documented_leaves = dict(leaves(sample))
    documented = set(documented_leaves)
    emitted = set(emitted_leaves)

    problems = []

    # A documented field that does not exist is a lie, whichever kind of
    # sample it is.
    for field in sorted(f for f in documented - emitted if f not in PENDING_REBUILD):
        problems.append(f"  - documents {field}, which is not emitted")

    # An omission only matters for a sample that claims to be complete. An
    # excerpt is allowed to be short; it is not allowed to be wrong.
    if kind == "complete":
        for field in sorted(emitted - documented):
            problems.append(f"  + omits {field}, which is emitted")

    for path in sorted(documented & emitted):
        documented_type = json_type(documented_leaves[path])
        actual_type = json_type(emitted_leaves[path])
        if documented_type != actual_type:
            problems.append(
                f"  ~ {path}: documented as {documented_type}, emitted as {actual_type}"
            )

    if "_schema" in documented & emitted and documented_leaves["_schema"] != emitted_leaves["_schema"]:
        problems.append(
            f'  ! _schema is a contract identifier and must match exactly: '
            f'documented {documented_leaves["_schema"]!r}, emitted {emitted_leaves["_schema"]!r}'
        )
    version_value = documented_leaves.get("_nebu_version")
    if version_value is not None and not VERSION_RE.match(str(version_value)):
        problems.append(
            f'  ! _nebu_version {version_value!r} is not version-shaped; use a real '
            f'released version, not a guess at the next one'
        )
    for path, ok in PENDING_SHAPE.items():
        if path in documented_leaves and not ok(documented_leaves[path]):
            problems.append(f"  ! {path} has an implausible documented value: {documented_leaves[path]!r}")

    return problems, documented


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

    emitted_leaves = dict(leaves(actual))

    failed = False
    pending_seen = set()
    for relative_path, kind in SAMPLES:
        problems, documented = check_sample(relative_path, kind, actual, emitted_leaves)
        pending_seen |= documented & PENDING_REBUILD
        if problems:
            failed = True
            print(f"{relative_path} ({kind}) does not match token-transfer output:", file=sys.stderr)
            for problem in problems:
                print(problem, file=sys.stderr)

    if failed:
        print(
            "\nUpdate the documented samples so the shop window matches the "
            "product. A sample listed as 'complete' must show every field; an "
            "'excerpt' may show fewer, but every field it shows must be real "
            "and correctly typed.",
            file=sys.stderr,
        )
        return 1

    note = f" ({', '.join(sorted(pending_seen))} pending registry rebuild)" if pending_seen else ""
    print(f"{len(SAMPLES)} documented samples match token-transfer output{note}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
