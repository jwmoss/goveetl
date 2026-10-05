# AGENTS.md

Working conventions for coding agents (and humans) on this repo. Read this
before adding code; follow the release process only when cutting a release.

**Read first, in order:**

1. `AGENTS.md` — this file. Rules for working in this repo.
2. `skills/govee-api/SKILL.md` — how to use/extend the device API surface
   (commands, safety for household devices, capability routing).
3. `skills/goveetl-release/SKILL.md` — how to tag + release, and common
   release-pipeline failure modes (asset collision, tap token).

Load a skill when the task matches it: device work → `govee-api`; tag/merge/
tagless release → `goveetl-release`.

## Purpose

`goveetl` is a Go CLI for Govee devices across four surfaces — official API,
app API, MQTT, LAN. Provider-specific behavior goes in `internal/govee/`;
commands in `internal/cli/` stay thin wrappers; `internal/api` and `internal/
output` stay generic (transport + formatting only, no Govee knowledge).

## Layout

```
cmd/goveetl/          main.go only
internal/api/         generic HTTP client (auth, dry-run, tracing)
internal/govee/       Govee wire types + per-backend clients
internal/cli/         cobra commands, thin over internal/govee
internal/config/      yaml config + env (prefix GOVEETL_)
skills/               agent skills (see "Working with agents" below)
```

## Getting started

```bash
make build          # ./bin/goveetl
make check          # fmt + tidy + vet + test + build + go.mod diff gate
```

`make check` is the pre-handoff gate; if it fails, fix before continuing. Its
`git diff --exit-code -- go.mod go.sum` step fails when you've left dependency
changes uncommitted — run `make tidy` first or commit the change.

## CLI rules

- Keep `--json` stable for scripts. Changes to key names are breaking.
- `--plain` is stable and line-oriented where implemented.
- Mutating commands must honor `--dry-run`.
- Never print secrets (tokens, API keys, passwords). Redact in logs.
- Never add password/token CLI flags; read from env, config (0600), or stdin.
- Errors and diagnostics go to stderr; data goes to stdout.
- Commands that need credentials explain the missing env/config key in the error.
- New commands wire in `internal/cli/root.go` and get a short `-h` string.

## API integration rules

- All endpoint/shape truth lives in `internal/govee/`. Never parse responses
  inside a command.
- When adding a command name, add a `capabilityRoute` mapping in
  `internal/govee/openapi.go` plus a test in
  `internal/govee/govee_test.go`. Route entries describe the officially
  documented `(capability type, instance)` pair; don't invent untested ones.
- Don't push changes that mutate another person's device state without a state
  snapshot + restore plan; see the safe-change discipline in
  `skills/govee-api/SKILL.md`.
- Secrets: keep API keys/tokens in 1Password or env; never in code, tests, or
  fixtures. Reference 1Password at runtime with `op item get`.

## Testing

- Unit test wire shapes (`internal/govee/govee_test.go`); that's where drift
  bites first.
- Don't write tests that hit Govee live — unit-test shapes, live-verify by hand.
- `make check` after every material change.

## Commits and PRs

- Commit syntax: conventional commits (`feat:`, `fix:`, `chore:`), imperative,
  subject <= 72 chars.
- Feature work lands on main through PRs — no direct pushes to main.
- One logical change per commit. Squash noisy WIP commits before opening the PR.
- Never amend or rebase commits already pushed to a shared branch.
- pin CI action versions by SHA with a version comment; keep them current
  (`actions/checkout@<sha> # vX.Y.Z`).

## Working with agents (skills)

This repo ships two built-in agent skills under `skills/`, discoverable by
agents in the repo root:

| Skill | Load when |
| --- | --- |
| `govee-api` | Any Govee device interaction or goveetl command work; extending capability routes |
| `goveetl-release` | Cutting a release, tagging, or troubleshooting the release/Homebrew pipeline |

When adding a repository capability that an agent needs to operate (new backend,
new command family, new pipeline), add or extend the matching skill. Keep each
SKILL.md focused: frontmatter `name`/`description` first, one job per skill, and
push detail into sections rather than new files. Entries in this table are the
prompt-load for the skill. Don't duplicate rules here that a skill already
covers — link it.

## Validation

```bash
make check
```

Release pipeline changes: also verify the workflow with `gh run watch` after
the tag push (see `skills/goveetl-release/SKILL.md` failure modes).
