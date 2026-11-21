# Discover processors

Before composing anything, a user asks nebu what exists: `nebu list` prints
the catalog of origins, transforms, and sinks, `nebu describe <name>` explains
one of them with its schema and examples, and `<processor> --describe-json`
gives a machine-readable envelope that agents and orchestrators read. The
catalog merges nebu's built-in `registry.yaml` with the external community
registry, so "it printed something" and "it printed the real catalog" are
different outcomes.

## Sub-features

- `discover-list` prints built-in and `[community]` processors grouped by
  type.
- `discover-list-degraded` warns on stderr and still exits 0 when the
  external registry cannot be reached.
- `discover-describe` prints type, source, version, schema id, and examples
  for one processor.
- `discover-describe-json` emits a valid describe envelope to stdout, exits 0,
  starts no processing, and needs no runtime flags.
- `discover-unknown` refuses an unknown name with exit code 3.

## How to get to it (user POV)

- `nebu list`
- `nebu describe token-transfer`
- `token-transfer --describe-json | jq .`
- `NEBU_REGISTRY=<url> nebu list` to point at a different external registry.

## Driving it with nebu-verify

Preconditions:

- `nebu-verify up --published` has completed and `doctor` passes.
- Network access to the external registry
  (`github.com/withObsrvr/nebu-processor-registry`).

- **List the catalog.** Run `nebu-verify run list -- nebu list`. Exit 0,
  stdout has `ORIGIN PROCESSORS`, `TRANSFORM PROCESSORS`, and
  `SINK PROCESSORS` sections.
- **Prove the community registry actually arrived.** Run
  `nebu-verify sh list-community -- "nebu list 2>/dev/null | grep -c '\[community\]'"`.
  Stdout is a count well above zero, and the `list` step's `.err` file is
  empty. A non-empty `.err` with a zero exit means the embedded catalog only.
- **Prove the cache is this run's.** Run
  `nebu-verify sh list-cache -- 'find "$XDG_CACHE_HOME/nebu/registries" -type f | wc -l'`.
  The count is non-zero and the path is inside the run root, so the catalog
  was not read from the developer's `~/.cache`.
- **Describe one processor.** Run
  `nebu-verify run describe-tt -- nebu describe token-transfer`. Exit 0 and
  stdout names `Type: origin`, `Source: community (external registry)`, and
  `Schema: nebu.token_transfer.v1`.
- **Read the machine envelope.** Run
  `nebu-verify sh describe-json -- "token-transfer --describe-json | jq -e '.name == \"token-transfer\" and .type == \"origin\" and .schema.id == \"nebu.token_transfer.v1\"'"`.
  Stdout is `true`.
- **Confirm the envelope starts no work.** Check that the `describe-json`
  step's `.err` file is empty — no ledger fetch, no RPC warning, and no
  required flag was needed to produce it.
- **Describe every installed processor.** Run
  `nebu-verify sh describe-json-all -- 'for p in $(cat "$NEBU_VERIFY_ROOT/processors.txt"); do "$p" --describe-json | jq -c "{name, type, version: (.version // \"none\"), schema: (.schema.id // \"none\")}"; done'`.
  One line per processor; keep it as the version record for the whole proof.
- **Refuse an unknown name.** Run
  `nebu-verify run describe-unknown -- nebu describe no-such-processor`. The
  `.exit` file reads `3`.
- **Proof.** `list.out`, `list.err` (must be empty), `list-community.out`,
  `describe-tt.out`, `describe-json-all.out`. The last one identifies every
  binary the rest of the proof used.

## Gotchas

- **`nebu list` exits 0 when the external registry is unreachable.** It prints
  `Warning: could not fetch external registry: …` to stderr and returns only
  the embedded entries (`pkg/registry/registry.go:236-239`). Checking the exit
  code alone turns a degraded catalog into a pass; check stderr and the
  `[community]` count.
- A stale cache also masks an outage: the external registry falls back to a
  stale cache and warns (`pkg/registry/external.go:185-188`). The
  empty-stderr assertion is what distinguishes a fresh fetch from a cached one.
- The registry cache honours `XDG_CACHE_HOME` (`pkg/registry/external.go:157`).
  Driving nebu outside the helper uses the developer's real cache, so a
  "registry works" result there proves nothing about a cold machine.
- A **sink's** describe envelope legitimately carries an empty `schema` object
  (`docs/PROCESSOR_CONTRACT.md:113`); `json-file-sink` reports
  `{"schema":{}}`. Requiring `.schema.id` for every processor fails a healthy
  installation.
- `nebu describe` renders the processor's README, which is long. Assert on
  specific lines (`Type:`, `Schema:`), never on total output size.
- Community processor availability changes without a nebu release. Only claim
  a processor exists after `nebu list` in *this* run showed it.
