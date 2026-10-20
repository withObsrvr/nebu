# Security policy

## Reporting a vulnerability

**Do not open a public issue for a security vulnerability.**

Report it privately through either channel:

- [GitHub private vulnerability reporting](https://github.com/withObsrvr/nebu/security/advisories/new)
  — preferred, because it keeps the report, the fix, and the advisory together.
- Email **security@withobsrvr.com**.

Please include: the nebu version (`nebu --version`), the platform, what the
vulnerability allows an attacker to do, and the smallest reproduction you have.
If a ledger range or a specific input triggers it, include it — nebu bugs are
often data-dependent, and the input is usually the fastest route to a fix.

### What to expect

| Stage | Target |
|---|---|
| Acknowledgement of your report | 3 business days |
| Initial assessment and severity | 10 business days |
| Fix released, or a dated plan if the fix is involved | 90 days |

nebu is maintained by a small team; these are targets, not contractual SLAs. If
you have not heard back within the acknowledgement window, please escalate by
emailing again — it means the first message went astray.

We will credit you in the advisory unless you ask us not to. We do not operate a
paid bug-bounty programme.

## Supported versions

nebu is pre-1.0. Security fixes land on the **latest minor release only**; there
are no long-term support branches. Upgrade to the current release before
reporting, in case the issue is already fixed.

## Scope

nebu is a local command-line tool and a Go library. It runs as the user who
invokes it, with that user's credentials, and exposes no network listener. The
security surface is accordingly narrow but not empty. In scope:

- **Untrusted ledger and event data.** nebu decodes XDR and JSON from Stellar
  RPC endpoints, history archives, object storage, and upstream pipeline
  stages. A crafted ledger, event, memo, asset code, or contract value that
  causes a crash, an infinite loop, unbounded memory growth, or a terminal
  escape-sequence injection (see CLI-22 in the
  [engineering profile](docs/engineering-profile.md)) is in scope.
- **Untrusted registry metadata.** `registry.yaml` and external
  `description.yml` documents drive `nebu install`, which runs `go install`
  against a module path. Metadata that causes nebu to fetch, build, or execute
  something other than the processor the user named is in scope and serious.
- **Credential handling.** Any path where nebu writes a credential to stdout, to
  a checkpoint file, into an event payload, into a status record, or into a
  terminal escape sequence is in scope. `NEBU_RPC_AUTH` is the supported way to
  pass RPC credentials.
- **Checkpoint integrity.** A checkpoint that causes nebu to silently skip or
  duplicate ledgers on resume is in scope — for a data pipeline, a wrong answer
  is a security-relevant failure (P-1).

Out of scope:

- Vulnerabilities in Stellar Core, Stellar RPC, history archives, or the
  `go-stellar-sdk`. Report those to their maintainers; we will help you route it
  if you are unsure.
- Processors published in
  [nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry).
  Report those against that repository. nebu owns the contract they implement,
  not their code.
- Anything requiring the attacker to already control the machine or the user
  account nebu runs as.
- Denial of service achieved only by pointing nebu at a slow or hostile RPC
  endpoint the user chose.

## Known gaps

We would rather state these than have you spend time rediscovering them. Both
are tracked in the [engineering profile](docs/engineering-profile.md):

- **Secrets can be passed as command-line arguments.** `--rpc-header` and
  `--dsn` accept values that are typically credentials. Process arguments are
  visible to every user on the host, which CLI-10 of the OBSRVR Engineering
  Standard forbids. Use `NEBU_RPC_AUTH` and `POSTGRES_DSN` instead. Removing the
  argument-borne path is planned.
- **`nebu` defaults to the Stellar public network.** BUILD-7 requires the network
  to be explicit configuration with no pubnet default. Until that is fixed, pass
  `--network` explicitly in anything automated.

## Supply chain

Dependencies are pinned (`go.sum`, `flake.lock`) and updated through reviewed
pull requests. Every pull request runs `govulncheck` and fails on reachable
known vulnerabilities, plus secret scanning on the diff. Release artifacts carry
checksums. Signing, SBOM attachment, and SLSA provenance are in progress — see
the profile for current status, and do not treat an unsigned artifact as
verified.
