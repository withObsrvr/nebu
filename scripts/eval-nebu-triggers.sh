#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/eval-nebu-triggers.sh [options]

Run trigger evals for the nebu Agent Skill. The script asks a fresh Pi session
whether each prompt should load the skill, records JSONL logs, and summarizes
trigger rates.

Options:
  --queries <file>       Eval queries JSON (default: skills/nebu/evals/trigger-evals.json)
  --skill <path>         Skill directory or SKILL.md (default: skills/nebu)
  --runs <n>             Runs per query (default: 3)
  --out <dir>            Output directory (default: skills/nebu/evals/runs/<timestamp>)
  --provider <name>      Pi provider to use (optional)
  --model <model>        Pi model to use (optional)
  --pi <bin>             Pi binary (default: pi)
  --dry-run              Print planned runs without calling Pi
  -h, --help             Show this help

Requires: pi, jq, base64.

Pass criteria:
  should_trigger=true  -> trigger_rate > 0.5
  should_trigger=false -> trigger_rate < 0.5
USAGE
}

QUERIES="skills/nebu/evals/trigger-evals.json"
SKILL="skills/nebu"
RUNS=3
OUT=""
PROVIDER=""
MODEL=""
PI_BIN="pi"
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --queries) QUERIES="$2"; shift 2 ;;
    --skill) SKILL="$2"; shift 2 ;;
    --runs) RUNS="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --provider) PROVIDER="$2"; shift 2 ;;
    --model) MODEL="$2"; shift 2 ;;
    --pi) PI_BIN="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ ! -f "$QUERIES" ]]; then
  echo "Missing queries file: $QUERIES" >&2
  exit 1
fi

if [[ -d "$SKILL" ]]; then
  SKILL_DIR="$SKILL"
  SKILL_FILE="$SKILL/SKILL.md"
else
  SKILL_FILE="$SKILL"
  SKILL_DIR="$(dirname "$SKILL")"
fi

if [[ ! -f "$SKILL_FILE" ]]; then
  echo "Missing skill file: $SKILL_FILE" >&2
  exit 1
fi

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v base64 >/dev/null || { echo "base64 is required" >&2; exit 1; }
if [[ "$DRY_RUN" -eq 0 ]]; then
  command -v "$PI_BIN" >/dev/null || { echo "Pi binary not found: $PI_BIN" >&2; exit 1; }
fi

if [[ -z "$OUT" ]]; then
  OUT="skills/nebu/evals/runs/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$OUT/logs"

COUNT="$(jq 'length' "$QUERIES")"
RESULTS_JSONL="$OUT/results.jsonl"
SUMMARY_JSON="$OUT/summary.json"
: > "$RESULTS_JSONL"

HARNESS='You are running an Agent Skills trigger evaluation. Treat the following user query as the task. Decide whether the available nebu skill is relevant. If it is relevant, use the read tool to load the nebu SKILL.md, then answer exactly SKILL_TRIGGERED on its own line. If it is not relevant, do not use tools and answer exactly SKILL_NOT_TRIGGERED on its own line. Do not solve the task.'

pi_args_base=(--mode json --no-session --no-skills --skill "$SKILL_DIR" --tools read)
[[ -n "$PROVIDER" ]] && pi_args_base+=(--provider "$PROVIDER")
[[ -n "$MODEL" ]] && pi_args_base+=(--model "$MODEL")

printf 'Running %s queries x %s runs\n' "$COUNT" "$RUNS"
printf 'Skill: %s\nOutput: %s\n' "$SKILL_DIR" "$OUT"

for i in $(seq 0 $((COUNT - 1))); do
  encoded="$(jq -c ".[$i]" "$QUERIES" | base64 | tr -d '\n')"
  query="$(printf '%s' "$encoded" | base64 -d | jq -r '.query')"
  expected="$(printf '%s' "$encoded" | base64 -d | jq -r '.should_trigger')"
  safe_id="$(printf '%02d' $((i + 1)))"

  for run in $(seq 1 "$RUNS"); do
    log="$OUT/logs/query-${safe_id}-run-${run}.jsonl"
    prompt="$HARNESS

User query:
$query"

    echo "[$safe_id/$COUNT run $run/$RUNS] expected=$expected :: $query"

    if [[ "$DRY_RUN" -eq 1 ]]; then
      triggered=false
      status="DRY_RUN"
      : > "$log"
    else
      set +e
      "$PI_BIN" "${pi_args_base[@]}" -p "$prompt" > "$log" 2> "$log.stderr"
      exit_code=$?
      set -e

      if [[ "$exit_code" -ne 0 ]]; then
        triggered=false
        status="ERROR"
      else
        # Primary signal: Pi used read on this skill's SKILL.md.
        # Fallback signal: final text marker is exactly SKILL_TRIGGERED.
        # Do NOT grep for SKILL_TRIGGERED: SKILL_NOT_TRIGGERED contains it.
        if jq -e '
          select(.type == "tool_execution_start"
            and .toolName == "read"
            and ((.args.path // "") | endswith("/nebu/SKILL.md")))
        ' "$log" >/dev/null 2>&1; then
          triggered=true
        elif jq -e '
          select(.type == "message_update"
            and .assistantMessageEvent.type == "text_end"
            and (.assistantMessageEvent.content // "" | gsub("^[[:space:]]+|[[:space:]]+$"; "") == "SKILL_TRIGGERED"))
        ' "$log" >/dev/null 2>&1; then
          triggered=true
        else
          triggered=false
        fi
        status="OK"
      fi
    fi

    passed=false
    if [[ "$expected" == "true" && "$triggered" == "true" ]]; then
      passed=true
    elif [[ "$expected" == "false" && "$triggered" == "false" ]]; then
      passed=true
    fi

    jq -cn \
      --argjson index "$i" \
      --argjson run "$run" \
      --arg query "$query" \
      --argjson should_trigger "$expected" \
      --argjson triggered "$triggered" \
      --argjson passed "$passed" \
      --arg status "$status" \
      --arg log "$log" \
      '{index:$index, run:$run, query:$query, should_trigger:$should_trigger, triggered:$triggered, passed:$passed, status:$status, log:$log}' \
      >> "$RESULTS_JSONL"
  done
done

jq -s --argjson runs "$RUNS" '
  group_by(.index) |
  map({
    index: .[0].index,
    query: .[0].query,
    should_trigger: .[0].should_trigger,
    runs: length,
    triggers: map(select(.triggered)) | length,
    trigger_rate: ((map(select(.triggered)) | length) / length),
    pass: (if .[0].should_trigger then
      (((map(select(.triggered)) | length) / length) > 0.5)
    else
      (((map(select(.triggered)) | length) / length) < 0.5)
    end),
    statuses: map(.status) | unique
  }) as $per_query |
  {
    generated_at: now | todate,
    runs_per_query: $runs,
    total_queries: ($per_query | length),
    passed_queries: ($per_query | map(select(.pass)) | length),
    failed_queries: ($per_query | map(select(.pass | not)) | length),
    pass_rate: (($per_query | map(select(.pass)) | length) / ($per_query | length)),
    per_query: $per_query
  }
' "$RESULTS_JSONL" > "$SUMMARY_JSON"

echo
echo "Summary: $SUMMARY_JSON"
jq '. | {total_queries, passed_queries, failed_queries, pass_rate}' "$SUMMARY_JSON"
echo
echo "Failures:"
jq -r '.per_query[] | select(.pass | not) | "- [\(.trigger_rate)] expected=\(.should_trigger) :: \(.query)"' "$SUMMARY_JSON"
