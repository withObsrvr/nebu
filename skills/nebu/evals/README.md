# nebu skill evals

There are two useful eval loops:

1. **Trigger evals** — does the agent load the nebu skill for the right prompts?
2. **Output evals** — when loaded, does the skill produce better pipeline commands than a baseline?

## Trigger evals

Prompts live in `trigger-evals.json`:

```json
{ "query": "...", "should_trigger": true }
```

Run:

```bash
scripts/eval-nebu-triggers.sh
```

Useful variants:

```bash
scripts/eval-nebu-triggers.sh --runs 5
scripts/eval-nebu-triggers.sh --model sonnet --runs 3
scripts/eval-nebu-triggers.sh --dry-run --runs 1
```

The script runs each query in a clean Pi session and records whether the agent loaded `skills/nebu/SKILL.md`. A prompt passes when:

- `should_trigger=true` and trigger rate is > 0.5
- `should_trigger=false` and trigger rate is < 0.5

Results are written under `skills/nebu/evals/runs/<timestamp>/` with raw JSONL logs, `results.jsonl`, and `summary.json`.

## Output evals

Create `evals.json` with realistic tasks and expected outputs. Run each task twice:

- `with_skill/` using the current nebu skill
- `baseline/` with no skill or a previous skill snapshot

Grade outputs against assertions such as:

- command is copy-pasteable
- uses bounded ledger ranges unless live streaming was requested
- runs `nebu list` / `nebu describe` / `--describe-json` before relying on processor details
- uses current fields such as `.transfer.assetCode`
- includes a verification command
- flags assumptions and unverified details

Aggregate pass rates in a benchmark file and iterate on `SKILL.md` only when failures reveal reusable instructions.
