# Nebu Skill

**Canonical URL:** https://nebu.withobsrvr.com/SKILL.md

Bootstrap skill for installing Nebu, discovering installed processors, and composing a first working Stellar data pipeline.

## Use this skill when

The user wants to:
- install Nebu
- extract Stellar ledger data
- filter or transform a Stellar event stream
- write Stellar events to a file or external sink

Do not use this skill when the user wants to:
- build a brand-new processor
- work with non-Stellar chains
- sign or submit transactions

## Core rule

**Always inspect the installed version before proposing a command.**

Never guess:
- processor names
- flags
- schema paths
- output shapes

The installed version is the source of truth.

## Execution loop

1. Clarify the goal:
   - what data?
   - what ledger range?
   - where should output go?
2. Discover installed processors:
   ```bash
   nebu list
   ```
3. Inspect every candidate processor:
   ```bash
   <processor> --describe-json | jq .
   ```
4. Choose:
   - one **origin**
   - zero or more **transforms**
   - zero or one **sink**
5. Compose a **bounded** pipeline first.
6. Return:
   - exact command
   - one-line explanation of each stage
   - one verification command
   - assumptions or unverified details

## Installation

```bash
git clone https://github.com/withObsrvr/nebu && cd nebu
make build-cli build-processors
export PATH="$PWD/bin:$PATH"
```

## Pipeline model

Every pipeline has this shape:

```text
origin -> transform -> transform -> sink
```

- **origin** emits JSONL events
- **transform** reads JSONL from stdin and writes JSONL to stdout
- **sink** reads JSONL from stdin and writes elsewhere

## Minimal examples

Inspect installed processors:

```bash
nebu list
token-transfer --describe-json | jq .
json-file-sink --describe-json | jq .
```

Extract token transfers from a bounded range:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200001
```

Write results to a file:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200010 \
  | json-file-sink --out /tmp/nebu.jsonl
```

Inspect with standard Unix tooling:

```bash
token-transfer --start-ledger 60200000 --end-ledger 60200010 | jq .
```

## Stop conditions

Stop and ask for clarification if:
- the data type is unclear
- the ledger range is unclear
- the sink or destination is unclear

Stop and hand off if:
- no installed processor can do the job
- the user needs a new processor
- the task is not about Stellar data extraction

## Response contract

When proposing a pipeline, include:
1. the exact shell command
2. a short explanation of each stage
3. one verification command
4. any assumptions

## Stability

Nebu stays agent-safe because its runtime contract is discoverable:
- `nebu list`
- `nebu describe <name>`
- `<processor> --describe-json`

`--describe-json` is part of Nebu's stable contract. See [`STABILITY.md`](https://nebu.withobsrvr.com/STABILITY.md).

## Additional skills

- [`nebu-pipeline-composer`](https://github.com/withObsrvr/nebu/blob/main/skills/pipeline-composer/SKILL.md) — detailed multi-stage pipeline composition
- `nebu-processor-builder` — planned
- `nebu-common-errors` — planned
