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
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path

# One module path per processor, and the escaped spelling the proxy's URLs
# use: it encodes an uppercase letter as "!" plus its lowercase form, so
# withObsrvr becomes with!obsrvr.
MODULE = "github.com/withObsrvr/nebu-processor-registry/processors/{}"
ESCAPED_MODULE = "github.com/with!obsrvr/nebu-processor-registry/processors/{}"
# /@latest, not /@v/latest: these modules carry no tags, and @v/list is
# empty for them, so the latest-version endpoint is the only one that
# answers with a pseudo-version.
#
# It is the fallback rather than the source of truth. The proxy caches this
# endpoint, and it kept answering with the previous commit for a while after
# the registry's main had moved on — long enough for this check to report a
# stale pin as current, which is the one thing it exists not to do. The
# registry's git HEAD does not lag.
PROXY = "https://proxy.golang.org/{}/@latest"

# The repository the processor modules live in, derived from their shared
# module path prefix.
REPO = "https://github.com/withObsrvr/nebu-processor-registry"

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


def main_head():
    """The commit main points at, straight from the repository.

    Returns None when git cannot answer, in which case the caller falls back
    to the proxy.
    """
    try:
        result = subprocess.run(
            ["git", "ls-remote", REPO, "refs/heads/main"],
            capture_output=True, text=True, timeout=30,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0 or not result.stdout.strip():
        return None
    return result.stdout.split()[0]


def version_at(processor, commit):
    """The pseudo-version one processor module has at a commit.

    This is also what makes the proxy index that version: a nested module is
    not fetched until something asks for it by path, which is why a repin can
    otherwise resolve the parent module and fail with "found, but does not
    contain package".
    """
    module = MODULE.format(processor)
    try:
        result = subprocess.run(
            ["go", "list", "-m", f"{module}@{commit}"],
            capture_output=True, text=True, timeout=120,
            env={**os.environ, "GOWORK": "off", "GOFLAGS": ""},
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    parts = result.stdout.split()
    return parts[1] if len(parts) >= 2 else None


def latest_version(processor):
    """The newest version the proxy will serve for one processor module."""
    url = PROXY.format(ESCAPED_MODULE.format(processor))
    with urllib.request.urlopen(url, timeout=20) as response:
        return json.load(response)["Version"]


def stamp_of(version):
    """The pseudo-version's commit timestamp, or None if it is a real tag."""
    match = STAMP.search(version)
    return match.group(1) if match else None


def repin_advice(version, processors):
    """What to do about a stale pin, including the priming step."""
    primes = "\n".join(
        f"     go list -m {MODULE.format(p)}@{version}" for p in processors[:2]
    )
    return f"""
Processors built from the pin are older than what a user installs, so the checks
that compare documentation against processor output are reading an old product.

  1. Set registry_version to {version} in the justfile.
  2. Prime the proxy for each nested module, or `go install` resolves the parent
     module and fails with "found, but does not contain package":
{primes}
     ... and the same for the rest of the list.
  3. just build-processors
  4. just docs-smoke -- expect it to find whatever drifted in between."""


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: check-registry-pin.py <justfile>")

    pin = pinned_version(sys.argv[1])
    pin_stamp = stamp_of(pin)
    processors = processors_of(sys.argv[1])

    # Ask git first. A pseudo-version ends in the first 12 characters of the
    # commit it was built from, so the pin names its own commit.
    head = main_head()
    if head:
        pinned_commit = pin.rsplit("-", 1)[-1]
        if head.startswith(pinned_commit):
            print(f"registry pin {pin} is main ({head[:12]})")
            return 0

        resolved = version_at(processors[0], head)
        if resolved is None:
            print(
                f"the pinned registry version is not main:\n"
                f"  pinned:  {pin}\n"
                f"  main is: {head[:12]}\n\n"
                "Resolving its pseudo-version failed, so repin by hand:\n"
                f"  go list -m {MODULE.format(processors[0])}@{head}",
                file=sys.stderr,
            )
            return 1

        resolved_stamp = stamp_of(resolved)
        if pin_stamp and resolved_stamp and resolved_stamp <= pin_stamp:
            # The pin is at or ahead of main: a local commit, or a reverted
            # one. Not the drift this checks for.
            print(f"registry pin {pin} is not behind main ({head[:12]})")
            return 0

        print("the pinned registry version is behind the registry's main:", file=sys.stderr)
        print(f"  pinned:  {pin}", file=sys.stderr)
        print(f"  main:    {resolved}", file=sys.stderr)
        print(repin_advice(resolved, processors), file=sys.stderr)
        return 1

    # No git: fall back to the proxy, which is a weaker signal.
    print("git could not reach the registry; falling back to the module proxy, which caches")

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
        print(repin_advice(newest, processors), file=sys.stderr)
        return 1

    print(f"registry pin {pin} is current across {len(processors)} processor modules")
    return 0


if __name__ == "__main__":
    sys.exit(main())
