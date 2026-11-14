#!/usr/bin/env python3
"""Report when the justfile's pinned registry version has fallen behind.

`build-processors` installs the published processors at one pinned version,
which is right: `@latest` is not a contract, and it had already gone wrong
once — Go resolved it against the pseudo-version list in a warm module cache,
so a developer kept installing pre-Protocol-30 processors with no error.

Pinning has its own failure mode, though, and it is quieter. Nothing prompts
anyone to advance the pin, so it sat two weeks and four merged registry PRs
behind, and `docs-smoke` spent that time comparing the documented samples
against processors built before `meta.closedAt` existed. Every check passed.
The checks were right; they were reading an old product.

So this one asks the module proxy what the newest published version of each
module `build-processors` installs is, and fails when any of them is newer
than the pin. It is a reminder with teeth, not a merge gate: docs-smoke is
not what CI enforces.

Usage: check-registry-pin.py <justfile>
"""

import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

# The proxy escapes an uppercase letter as "!" plus its lowercase form, so
# withObsrvr is with!obsrvr. Asking the proxy directly also sidesteps the
# warm-cache resolution that made @latest unreliable in the first place.
MODULE = "github.com/with!obsrvr/nebu-processor-registry/processors/{}"
# /@latest, not /@v/latest: these modules carry no tags, and @v/list is
# empty for them, so the latest-version endpoint is the only one that
# answers with a pseudo-version.
PROXY = "https://proxy.golang.org/{}/@latest"

PIN = re.compile(r'^registry_version\s*:=\s*"([^"]+)"', re.MULTILINE)
# The processor list is read out of the recipe rather than copied here, so
# adding a processor to build-processors cannot leave this check behind.
# The recipe runs to the next unindented line, which is the next recipe or
# the end of the file. Stopping at the first `echo` would cut the body off
# above the loop.
RECIPE = re.compile(r"^build-processors:.*?(?=^\S|\Z)", re.MULTILINE | re.DOTALL)
FOR_LOOP = re.compile(r"for p in ([^;]+); do", re.DOTALL)
# A pseudo-version carries the commit's UTC timestamp: v0.0.0-YYYYMMDDHHMMSS-abcdef123456
STAMP = re.compile(r"\d+\.\d+\.\d+-(\d{14})-")


def pinned_version(justfile):
    match = PIN.search(Path(justfile).read_text(encoding="utf-8"))
    if not match:
        sys.exit(f"no registry_version assignment found in {justfile}")
    return match.group(1)


def processors_of(justfile):
    """The processors build-processors installs, read from the recipe."""
    text = Path(justfile).read_text(encoding="utf-8")
    recipe = RECIPE.search(text)
    if not recipe:
        sys.exit(f"no build-processors recipe found in {justfile}")
    loop = FOR_LOOP.search(recipe.group(0))
    if not loop:
        sys.exit(f"build-processors in {justfile} no longer loops over a processor list")
    names = loop.group(1).replace("\\\n", " ").split()
    if not names:
        sys.exit(f"build-processors in {justfile} installs no processors")
    return names


def latest_version(processor):
    """The newest version the proxy will serve for one processor module."""
    url = PROXY.format(MODULE.format(processor))
    with urllib.request.urlopen(url, timeout=20) as response:
        return json.load(response)["Version"]


def stamp_of(version):
    """The pseudo-version's commit timestamp, or None if it is a real tag."""
    match = STAMP.search(version)
    return match.group(1) if match else None


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: check-registry-pin.py <justfile>")

    pin = pinned_version(sys.argv[1])
    pin_stamp = stamp_of(pin)
    processors = processors_of(sys.argv[1])

    stale = []
    for processor in processors:
        try:
            latest = latest_version(processor)
        except (urllib.error.URLError, TimeoutError, KeyError, json.JSONDecodeError) as err:
            # Say so rather than pass quietly: an unreachable proxy means this
            # check ran and learned nothing, which is not the same as agreeing.
            print(f"skipped: the module proxy did not answer for {processor} ({err})")
            return 0
        if latest == pin:
            continue
        latest_stamp = stamp_of(latest)
        # A pin ahead of the proxy is fine — a commit it has not indexed yet.
        # Only a pin behind is the drift this exists to catch. Tagged versions
        # have no timestamp to compare, so any difference counts.
        if pin_stamp and latest_stamp and latest_stamp <= pin_stamp:
            continue
        stale.append((processor, latest))

    if stale:
        print("the pinned registry version is behind what the proxy publishes:", file=sys.stderr)
        print(f"  pinned:  {pin}", file=sys.stderr)
        # A registry bump touches every module, so one line per module says
        # the same thing ten times. Group by version instead.
        by_version = {}
        for processor, latest in stale:
            by_version.setdefault(latest, []).append(processor)
        for latest, names in sorted(by_version.items()):
            print(f"  newer:   {latest}  ({', '.join(names)})", file=sys.stderr)
        newest = max(by_version)
        print(
            "\nProcessors built from the pin are older than what a user installs, so the "
            "checks that compare documentation against processor output are reading an old "
            f"product. Repin registry_version to {newest} in the justfile, run "
            "`just build-processors`, and rerun docs-smoke — expect it to find whatever "
            "drifted in between.",
            file=sys.stderr,
        )
        return 1

    print(f"registry pin {pin} is current across {len(processors)} processor modules")
    return 0


if __name__ == "__main__":
    sys.exit(main())
