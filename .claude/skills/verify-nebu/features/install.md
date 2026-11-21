# Install the CLI and a processor

A new user installs the published `nebu` binary with `go install`, puts it on
`PATH`, installs one processor by name, and sees schema-versioned JSONL on
stdout. This is the path the README prints and the one the website quickstart
links to, so it must work verbatim against the module proxy — not against a
local build of this checkout.

## Sub-features

- `install-cli` installs `nebu` from the module proxy and reports its version.
- `install-processor` installs a community processor by name through
  `nebu install`, landing a runnable binary on `PATH`.
- `install-lands-where-path-says` confirms the installed binary exists at the
  location `command -v` resolves, not merely where nebu claimed.
- `install-first-events` runs the quickstart's final command and produces
  non-empty, schema-stamped JSONL.
- `install-unknown` refuses an unknown processor name with exit code 3 and
  lists what is available.

## How to get to it (user POV)

- `go install github.com/withObsrvr/nebu/cmd/nebu@latest` then
  `export PATH="$HOME/go/bin:$PATH"` (README "Quick Start").
- `nebu install token-transfer`, then run `token-transfer` directly.
- `nebu install <name> --path /usr/local/bin` for a custom directory.
- `just install` from a clone (developer path, not the shipped artifact).
- `docker run --rm withobsrvr/nebu:latest --version` (not mapped yet).

## Driving it with nebu-verify

Preconditions:

- Network access to `proxy.golang.org` and to a Stellar RPC endpoint.
- No run currently up (`nebu-verify where` fails, or the previous run was
  taken `down`).

- **Install the CLI.** `go install …@latest` into an empty `GOBIN`. Run
  `nebu-verify up --published`. The step `001-install-nebu` exits 0 and
  `nebu --version` prints `nebu version vX.Y.Z`; that version string is the
  identity of everything proved afterwards.
- **Confirm the installation is ours.** Run `nebu-verify doctor`. It passes
  `nebu resolves inside the run root` — a `nebu` from `~/go/bin` means the
  run root was not first on `PATH` and nothing below is a proof about the
  shipped artifact.
- **Install a processor.** `up` runs `nebu install token-transfer` for you
  from the run root. The step exits 0, and `up` fails loudly if the binary is
  absent from `GOBIN` afterwards.
- **Prove the binary is really there.** Run
  `nebu-verify run which-tt -- bash -c 'command -v token-transfer && token-transfer --version'`.
  The resolved path is under the run root's `bin/`, and the version prints.
- **Run the quickstart's final command verbatim.** Run
  `nebu-verify run quickstart-verbatim -- token-transfer --start-ledger 60200000 --end-ledger 60200001`.
  Exit code 0 and stdout holds many JSONL lines; each carries `_schema` and
  `_nebu_version`.
- **Assert the output, not the exit code.** Run
  `nebu-verify sh quickstart-assert -- "jq -se 'length > 0 and all(.[]; ._schema == \"nebu.token_transfer.v1\" and ._nebu_version != null)' '$E/steps/'*quickstart-verbatim.out"`.
  Stdout is `true`.
- **Refuse an unknown name.** Run
  `nebu-verify run install-unknown -- nebu install no-such-processor`. The
  `.exit` file reads `3` and stderr names the processor and lists available
  ones.
- **Proof.** The surviving evidence is
  `.verify-artifacts/<run-id>/steps/001-install-nebu.*`, the `install-*`
  steps, `quickstart-verbatim.out`, and `quickstart-assert.out`. State the
  `nebu --version` value and that the mode was `published`.

## Gotchas

- `nebu install` prefers a **local build** when the registry entry's
  `Location.Path` exists relative to the current directory
  (`cmd/nebu/install.go:99-104`). Run from inside this repo and you install
  your working tree while believing you installed the release. Every helper
  step uses the run root as cwd for exactly this reason.
- `--path` is never passed through to `go install` for a community processor
  (`cmd/nebu/install.go:158-186`): the command runs `go install <pkg>@latest`,
  `GOBIN` decides the destination, and nebu prints `Installed: <--path>/<name>`
  either way. The printed line is not evidence; the file on disk is.
- `@latest` for a registry processor resolves a pseudo-version, and Go can
  resolve it from a warm local module cache — silently installing a stale
  binary with no error (see the `registry_version` comment in `justfile`).
  Record the installed version from `--describe-json`.
- The published processors can be ahead of the pin `just build-processors`
  installs. A flag that works on `bin/token-transfer` may be absent from the
  published one, and the reverse.
- `token-transfer --start-ledger …` with no fixture hits live RPC. Run it once
  per session; the endpoint 429s on the third `getLedgers` call in a burst.
- The documented `export PATH="$HOME/go/bin:$PATH"` is deliberately *not*
  reproduced here — the harness installs into its own `GOBIN` so a verification
  run never shadows or is shadowed by the developer's own installation. A claim
  about the literal `$HOME/go/bin` instruction is therefore unverified by this
  recipe; say so.
