# goveetl

[![CI](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml/badge.svg)](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jwmoss/goveetl)](https://github.com/jwmoss/goveetl/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Command-line client for Govee cloud, app, and LAN APIs.

## Install

### Go

```bash
go install github.com/jwmoss/goveetl/cmd/goveetl@latest
```


### Homebrew

```bash
brew tap jwmoss/tap
brew install jwmoss/tap/goveetl
```


### Source

```bash
git clone https://github.com/jwmoss/goveetl.git
cd goveetl
make build
./bin/goveetl version
```

## Configuration

Initialize a config file:

```bash
goveetl config init --base-url https://openapi.api.govee.com
```

Config path:

```text
$XDG_CONFIG_HOME/goveetl/config.yaml
```

Environment variables:

| Variable | Purpose |
| --- | --- |
| `GOVEETL_BASE_URL` | API base URL |
| `GOVEETL_TOKEN` | API token |
| `GOVEETL_AUTH_HEADER` | Auth header name |
| `GOVEETL_AUTH_SCHEME` | Auth scheme, for example `Bearer` |

Precedence:

```text
flags > environment > config file > defaults
```

Tokens are intentionally not accepted as command-line flags by default. Use the
environment, a `0600` config file, or stdin-backed setup.

## Usage

```bash
goveetl --help
goveetl --version
goveetl version
goveetl doctor
goveetl devices list
goveetl devices get 123
goveetl raw GET /v1/me --json
printf '%s\n' "$TOKEN" | goveetl config init --token-stdin --force
goveetl completion zsh > ~/.zfunc/_goveetl
```

## Global Flags

| Flag | Description |
| --- | --- |
| `--config` | Config file path |
| `--base-url` | API base URL override |
| `--version` | Print version information |
| `--json` | Emit JSON to stdout |
| `--plain` | Emit stable plain text where available |
| `--quiet`, `-q` | Suppress non-essential output |
| `--no-color` | Disable color |
| `--timeout` | HTTP timeout |
| `--trace-http` | Log HTTP method/path/status to stderr |
| `--dry-run` | Refuse non-GET HTTP requests |
| `--no-input` | Disable interactive prompts |

## Exit Codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Runtime error |
| 2 | Invalid usage |

## Development

```bash
make check
```

## Release

Tag a semver release:

```bash
git tag v0.1.0
git push origin main
git push origin v0.1.0
```

The release workflow uses GoReleaser to publish archives and checksums.
Set `HOMEBREW_TAP_TOKEN` before the first tagged release so GoReleaser can
update `jwmoss/homebrew-tap`.
