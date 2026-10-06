# goveetl

[![CI](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml/badge.svg)](https://github.com/jwmoss/goveetl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jwmoss/goveetl)](https://github.com/jwmoss/goveetl/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Command-line client for Govee cloud, app, MQTT, and LAN APIs.

| Backend | Features | Authentication |
| --- | --- | --- |
| `api` | Device list, state, capabilities, control, dynamic scenes, DIY scene lists | Developer API key |
| `app` | Device lists, groups, and saved automations through `app2.govee.com` | App session from `auth login` or an imported token |
| `mqtt` | Device control and state subscriptions through AWS IoT | App session and IoT certificate |
| `lan` | Local discovery, state, and control through plaintext UDP | Enable LAN Control in Govee Home |

## Install

```bash
go install github.com/jwmoss/goveetl/cmd/goveetl@latest
```

Or build from source:

```bash
git clone https://github.com/jwmoss/goveetl.git
cd goveetl
make build
./bin/goveetl version
```

## Official API

Store the API key through stdin. Replace the example device reference with a
real `<device>:<sku>` value from the device list. The separator is the last colon.

```bash
printf '%s' "$GOVEE_API_KEY" | goveetl config set api_key --stdin
goveetl devices list --json
goveetl devices state 'AA:BB:H706C'
goveetl devices capabilities 'AA:BB:H706C'
goveetl control 'AA:BB:H706C' brightness 60
goveetl control 'AA:BB:H706C' colorTemp 2700
goveetl control 'AA:BB:H706C' turn 1
```

Read state before a test. Restore the original state after the test.
`--dry-run` refuses mutations before authentication or network access.

```bash
goveetl control 'AA:BB:H706C' brightness 65 --dry-run
```

The refusal returns exit code 1. Read commands still work with `--dry-run`,
including official state and scene requests that use HTTP POST.

## Scenes and DIY

Discover the values for the selected device before control:

```bash
goveetl scenes 'AA:BB:H706C' --json
goveetl scenes 'AA:BB:H706C' --diy --json
```

Use the returned capability type, instance, and value with `control`.
For example, a dynamic scene value can contain `id` and `paramId`:

```bash
goveetl control 'AA:BB:H706C' lightScene '{"id":10621,"paramId":17813}'
```

Scene values differ by device. The example IDs are not universal.
The API can omit the active scene ID from state responses. Save the original
scene selection before a scene test; color and brightness alone cannot restore it.

## App session and MQTT

Sign in with your Govee account email. The CLI prompts for your password without echo.
If Govee requires email verification, the CLI requests a code and prompts for it.
The CLI verifies the new session before saving it. It does not store your password or code.

```bash
goveetl auth login --email you@example.com
goveetl auth status --json
```

Later logins reuse the saved email. Run `goveetl auth login` to renew a session.
For scripts, provide `GOVEETL_PASSWORD` or read the password from stdin:

```bash
printf '%s' "$GOVEETL_PASSWORD" | goveetl auth login --email you@example.com --stdin --no-input
```

Use `GOVEETL_VERIFICATION_CODE` when a script must supply an email code.
`--no-input` refuses prompts and reports the missing input. Failed logins preserve the saved session.
The CLI writes credentials to a private configuration file.

Session import remains available for an existing Govee Home session or integration:

```bash
printf '%s' "$GOVEE_TOKEN" | goveetl config set token --stdin
printf '%s' "$GOVEE_ACCOUNT_TOPIC" | goveetl config set account_topic --stdin
printf '%s' "$GOVEE_ACCOUNT_ID" | goveetl config set account_id --stdin
goveetl doctor --json
goveetl devices list --backend app --json
goveetl groups list --json
goveetl groups devices 42 --json
goveetl mqtt cert
goveetl mqtt topic --device 'AA:BB' --sku H706C
goveetl mqtt watch --duration 30s
```

The CLI creates a client ID on the first app request. If you run a watcher and
control command concurrently, give the watcher a separate `GOVEETL_CLIENT_ID`.
AWS MQTT clients with the same ID can disconnect each other.

MQTT requires a JSON object for command data. The command version defaults to 0;
use `--cmd-version` when the device requires another version.

```bash
goveetl control --backend mqtt 'AA:BB:H706C' brightness '{"val":65}'
```

App tokens can expire. Run `auth login` again when authentication fails.
`auth refresh` requires a refresh token. Govee rejects refresh for the verified login
with status 401; use `auth login` to renew the session. Automatic refresh is not implemented.
`auth logout` clears the stored session.

## Govee Home automations

Read automation IDs and saved device actions through the app session:

```bash
goveetl automations list --json
goveetl automations show 42 --json
```

The output includes power, brightness, temperature, and scene names when available.
It omits MQTT topics, command messages, and trigger location data.

Live verification covers H706C saved warm-white settings at 60% and 2700 K.
The CLI omits null object fields to match the app's serializer; including those
fields causes Govee status 500. Device-removal writes remain unverified live.

```bash
goveetl automations set 42 'AA:BB:H706C' --power on --brightness 60 --temperature 2700 --dry-run
goveetl automations set 42 'AA:BB:H706C' --power on --brightness 60 --temperature 2700
goveetl automations remove-device 42 'CC:DD:H616C'
goveetl automations show 42 --json
```

Power and brightness edits preserve the existing light mode. A temperature edit
replaces saved color or scene commands with native warm white. Temperature writes
currently support H706C at 2700 K only. Other devices and action groups remain unchanged.
The CLI refuses ambiguous device actions and removal of an action group's last device.
It reads the saved automation again after an accepted write and fails if verification differs.

## LAN

Enable LAN Control in Govee Home. Discovery sends JSON to multicast
`239.255.255.250:4001`. Devices reply on UDP 4002. State and control use UDP 4003.
The deprecated `lan_key` setting is ignored.

```bash
goveetl lan discover --json
goveetl lan discover --address 192.0.2.10 --json
goveetl lan status 192.0.2.10
goveetl lan control 192.0.2.10 brightness '{"value":65}'
goveetl lan control 192.0.2.10 turn '{"value":1}'
```

Use `--address` when multicast does not reach the device. Run one LAN command
at a time because the protocol uses a fixed reply port. A control result of
`sent` confirms transmission; use `lan status` to verify the device state.
A status request fails if the device does not reply.

## Configuration

POSIX config files use mode `0600`. Windows inherits permissions from the config directory.
The default path follows the operating system:

- macOS: `~/Library/Application Support/goveetl/config.yaml`
- Linux: `$XDG_CONFIG_HOME/goveetl/config.yaml`, or `~/.config/goveetl/config.yaml`

Use `--config` for another path. Precedence: flags, environment, config file, defaults.

`config show` reports the selected path. `config init --json` returns the path and status.
Config and session writes use a private temporary file, then replace the selected file.
The CLI rejects symbolic links and other non-regular destination files.
Without `--force`, config initialization cannot replace a file that another process creates.

Keys: `base_url`, `openapi_base_url`, `device_base_url`, `api_key`, `token`,
`refresh_token`, `account_topic`, `account_id`, `client_id`, `email`, `iot_version`.
Environment variables use uppercase keys with the `GOVEETL_` prefix.

The app host defaults to `https://app2.govee.com`. The loader corrects the
incorrect official host saved by v1.0.0. Explicit environment and flag overrides
remain available.

## Raw requests

`raw` uses the configured host and token. Add `--backend app` for app identity
headers and Govee response-status checks. The default retains generic HTTP behavior.

```bash
goveetl raw GET /bff-app/v1/device/list --backend app
goveetl raw GET /bff-app/v1/general-control/list --backend app --query filterEmpty=false
```

`--dry-run` blocks every non-GET raw request.

Raw JSON preserves large integers, duplicate keys, and exponent notation.
Use one of `--data` or `--file` for a JSON request body.
With `--json`, a non-JSON response returns exit code 1 with no response on stdout.

The client sends credentials and extra request headers only to the configured origin.
The origin includes the scheme, hostname, and port.
The configured device host has a separate credential scope for MQTT topic requests.
The client refuses redirects to another origin. Login and email verification refuse all redirects.
Errors and HTTP traces redact known credentials. HTTP responses have a 64 MiB limit.

## Verification and limits

Live verification covers password login with email verification, saved-session access,
official/app inventories, group membership, scene/DIY
catalogs, LAN discovery/status, MQTT connection/state messages, and brightness
control through cloud, LAN, and MQTT. Each control test restores the original state.

Automation device removal, group scene mutations, token refresh, and other reverse-engineered endpoints
remain unverified. The extracted APK endpoint list is research evidence, not a
claim that every endpoint has a supported CLI command.

Private API evidence comes from Govee Home 7.6.21. Use the tool with devices you own.

## Global flags

`--config`, `--base-url`, `--version`, `--json`, `--plain`, `--quiet`, `--no-color`,
`--timeout`, `--trace-http`, `--dry-run`, `--no-input`.

Exit codes: 0 for success, 1 for runtime errors or dry-run refusal, 2 for invalid usage.

Use one of `--json` or `--plain`. Commands reject extra positional arguments.
`--timeout` must be positive. It applies to app, account, official API, MQTT, and LAN operations.
LAN waits also honor `--wait`. MQTT watch uses `--timeout` for connection setup and `--duration` for the stream.
`--trace-http` covers each HTTP client and writes diagnostics to stderr.

The root `--version` flag works without a valid config file.
Version output uses module and VCS build information when release metadata is absent.

## Development and release

```bash
make check
```

Unit tests use local HTTP, MQTT, and UDP fixtures. Live device tests remain manual.
Release instructions are in [skills/goveetl-release/SKILL.md](skills/goveetl-release/SKILL.md).
