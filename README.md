# goveetl

[![CI](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml/badge.svg)](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jwmoss/goveetl)](https://github.com/jwmoss/goveetl/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Command-line client for Govee cloud, app, and LAN APIs.

`goveetl` works against three surfaces:

| Backend | What it is | Auth |
| --- | --- | --- |
| `api` | The official Govee OpenAPI (`openapi.api.govee.com`): device list, state, capabilities, control | `Govee-API-Key` from the Govee developer account |
| `app` | The private REST API the Govee Home app drives (`app.govee.com/bff-app/...`), discovered by reverse-engineering Govee Home 7.6.21 | Bearer token from `goveetl auth login` |
| `mqtt` | The app's AWS IoT control channel (mutual-TLS cert from the app API, write envelopes per the app's `Write` format) | App session |
| `lan` | The published Govee LAN API: UDP discovery (4002) and AES-128-ECB control (4001) | none |

## Install

### Go

```bash
go install github.com/jwmoss/goveetl/cmd/goveetl@latest
```

### Source

```bash
git clone https://github.com/jwmoss/goveetl.git
cd goveetl
make build
./bin/goveetl version
```

## Quickstart

Official API (key from the Govee developer portal):

```bash
goveetl config set api_key <GOVEE_API_KEY>
goveetl devices list --backend api
goveetl devices state H6123:12A3
goveetl control H6123:12A3 turn 1
goveetl devices capabilities H6123:12A3
```

Govee Home app API (no developer key required, but account credentials are used
once at login):

```bash
goveetl auth login --email you@example.com
goveetl devices list --backend app
goveetl groups list
goveetl groups devices 42
goveetl mqtt cert                  # endpoint + certificate fingerprints
goveetl mqtt topic --device 12A3 --sku H6123
goveetl control --backend mqtt --cmd-version 1 H6123:12A3 turn 1
```

Local LAN:

```bash
goveetl lan discover
goveetl lan control 192.0.2.10 on 1
```

Escapes:

```bash
goveetl raw POST /bff-app/v1/device/list --data '{}'
goveetl raw GET /bff-app/v1/general-control/list --query filterEmpty=false
```

## Configuration

Config path: `$XDG_CONFIG_HOME/goveetl/config.yaml` (0600).

Keys: `base_url` (app API), `openapi_base_url`, `device_base_url`, `api_key`,
`token`, `refresh_token`, `account_topic`, `account_id`, `client_id`, `email`,
`iot_version`, `lan_key`.

Environment variables (prefix `GOVEETL_`): `GOVEETL_API_KEY`, `GOVEETL_TOKEN`,
`GOVEETL_REFRESH_TOKEN`, `GOVEETL_BASE_URL`, `GOVEETL_ACCOUNT_TOPIC`,
`GOVEETL_ACCOUNT_ID`, `GOVEETL_CLIENT_ID`, `GOVEETL_EMAIL`, `GOVEETL_PASSWORD`
(read by `auth login`, never stored), `GOVEETL_LAN_KEY`, and the endpoint
overrides.

Precedence: flags > environment > config file > defaults.

Passwords and tokens are never accepted as command-line flags and are never
printed. `--dry-run` blocks all non-GET requests.

## Private API provenance

Endpoints, headers, login flow, the MQTT write envelope, and the `clientId`
format come from decompiling Govee Home 7.6.21 (com.govee.home). Extracted
evidence lives in the project notes (`../Govee_Goveetl/evidence`). All discovered
service paths (1274) are listed there; `goveetl raw` reaches any of them.

This tool is for interoperability with devices the user owns.

## Global Flags

| Flag | Description |
| --- | --- |
| `--config` | Config file path |
| `--base-url` | App API base URL override |
| `--version` | Print version information |
| `--json` | Emit JSON to stdout |
| `--plain` | Emit stable plain text where available |
| `--quiet`, `-q` | Suppress non-essential output |
| `--no-color` | Disable color |
| `--timeout` | HTTP timeout |
| `--dry-run` | Refuse non-GET HTTP requests |

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
git tag v0.1.0 && git push origin v0.1.0
```

The release workflow uses GoReleaser. Set `HOMEBREW_TAP_TOKEN` before the first
tagged release so GoReleaser can update `jwmoss/homebrew-tap`.
