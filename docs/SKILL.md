# Nebu Skills

**Canonical URL:** https://nebu.withobsrvr.com/SKILL.md

**Building-block Stellar data pipelines for AI agents.**

Nebu is a toolkit for composing Stellar ledger extraction pipelines from small, independently-runnable Unix processes. Every nebu binary supports `--describe-json` for runtime introspection, so agents can discover what's available at *this* version instead of relying on stale docs.

This page catalogs the Claude Code skills maintained alongside nebu. Each skill is a focused, installable instruction bundle that teaches an agent how to use one slice of the nebu ecosystem.

## Installation

Nebu skills are standard Claude Code skills — drop them into `~/.claude/skills/`:

```bash
git clone --depth 1 https://github.com/withObsrvr/nebu /tmp/nebu-skills
mkdir -p ~/.claude/skills
cp -r /tmp/nebu-skills/skills/* ~/.claude/skills/
rm -rf /tmp/nebu-skills
```

After install, agents using Claude Code can invoke any skill by name.

You also need the `nebu` CLI on your `PATH`. Build from source:

```bash
git clone https://github.com/withObsrvr/nebu && cd nebu
make build-cli build-processors
export PATH="$PWD/bin:$PATH"
```

## Catalog

| Skill | Status | What it does |
|---|---|---|
| [`nebu-pipeline-composer`](https://github.com/withObsrvr/nebu/blob/main/skills/pipeline-composer/SKILL.md) | **stable** | Compose multi-stage Stellar ledger pipelines (origin → transform → sink) from existing processors. Discovers the catalog via `nebu list` and the flags of each stage via `--describe-json`. |
| `nebu-processor-builder` | planned | Scaffold new processors from scratch (origin/transform/sink) with proto-first structure and CLI helper wiring. |
| `nebu-common-errors` | planned | Diagnose common nebu pipeline failures (XDR decode errors, schema drift, stdin auto-detect quirks). |

## Why these skills stay correct across releases

Most tool-wrapping skills rot on every release because they hardcode flag names and command syntax. Nebu skills defer to runtime discovery instead:

- `nebu list` — enumerate installed processors (in-tree + community registry)
- `nebu describe <name>` — human-readable description
- `<processor> --describe-json` — machine-readable envelope with the exact flags, schema, type, and version for *this* installed build

A skill that teaches the agent to always run describe first is durable across nebu releases. The `--describe-json` protocol is part of nebu's stable contract — see [`STABILITY.md`](https://nebu.withobsrvr.com/STABILITY.md).

## Foundation

Nebu and its skills are maintained by [OBSRVR](https://github.com/withObsrvr) as MIT-licensed open source.

## Contributing

New skill ideas welcome. Open an issue at [github.com/withObsrvr/nebu/issues](https://github.com/withObsrvr/nebu/issues) or send a PR adding a `skills/<skill-name>/SKILL.md` alongside an entry in the catalog table above.
