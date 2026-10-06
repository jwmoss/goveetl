---
name: govee-api
description: >-
  Use goveetl for Govee device reads and control through the official API, app
  API, MQTT, or LAN. Also use when extending device commands or debugging API
  contracts and session setup.
license: MIT
---

# Govee control through goveetl

Read `AGENTS.md` before code changes. Prefer the official API for supported operations.

| Surface | Credentials | Live verification |
| --- | --- | --- |
| Official | Developer API key | Inventory, state, scenes/DIY lists, brightness control |
| App | Captured session token | Device/group lists, group membership, automation reads |
| MQTT | App session, account ID and topic | Certificate, device topic, connection, state messages, brightness control |
| LAN | Enable LAN Control in Govee Home | Multicast/direct discovery, state, brightness control |

## Setup

Resolve credentials at runtime. The user's 1Password item `Govee` contains
`API_KEY`. Pass secrets through stdin or environment variables.

```bash
printf '%s' "$GOVEE_API_KEY" | goveetl config set api_key --stdin
goveetl doctor --json
```

Password login is unsupported. Import a session captured from the user's app
or an authorized local integration. Keep tokens out of output and evidence files.

```bash
printf '%s' "$GOVEE_TOKEN" | goveetl config set token --stdin
printf '%s' "$GOVEE_ACCOUNT_ID" | goveetl config set account_id --stdin
printf '%s' "$GOVEE_ACCOUNT_TOPIC" | goveetl config set account_topic --stdin
```

The app host is `https://app2.govee.com`. A successful doctor result has status
200 and no error for the selected backend. Reachability alone does not prove authentication.

## Device control

1. Read inventory and capabilities.
2. Read the target's current state.
3. Define how to restore the original state.
4. Apply one change.
5. Verify a fresh device state.
6. Restore the original state and verify it again.

```bash
goveetl devices list --json
goveetl devices state '<device>:<sku>'
goveetl control '<device>:<sku>' brightness 65 --dry-run
goveetl control '<device>:<sku>' brightness 65
```

Device IDs contain colons. The reference separator is the last colon.
Dry-run refuses mutations with exit code 1, including config and session changes.

## Scenes

```bash
goveetl scenes '<device>:<sku>' --json
goveetl scenes '<device>:<sku>' --diy --json
```

Use the returned capability type, instance, and value with `control`.
Scene IDs differ by device. The state response can omit the active scene ID.
Only test a scene when its original selection is known or the light uses a
restorable static color. State acceptance alone does not prove a visible effect.

## LAN

```bash
goveetl lan discover --json
goveetl lan discover --address 192.0.2.10 --json
goveetl lan status 192.0.2.10
goveetl lan control 192.0.2.10 brightness '{"value":65}'
```

LAN uses plaintext JSON under `msg`. Discovery uses multicast port 4001;
replies use local port 4002; control uses device port 4003. No AES key is required.
Run LAN commands sequentially. A `sent` result confirms transmission only.

## MQTT

```bash
goveetl mqtt topic --device '<device>' --sku '<sku>'
goveetl control '<device>:<sku>' brightness '{"val":65}' --backend mqtt
goveetl mqtt watch --duration 15s
```

Data must be a JSON object. Command versions differ by device; the default is 0.
Use distinct `GOVEETL_CLIENT_ID` values for concurrent watcher and control processes.
The device publish topic differs from the account reply topic. Preserve both.

## Saved automations

Use the CLI for household changes. Add missing operations to goveetl before use.
Use Java or decompiled code to establish the wire contract when needed.

```bash
goveetl automations list --json
goveetl automations show 42 --json
goveetl automations set 42 'AA:BB:H706C' --power on --brightness 60 --temperature 2700 --dry-run
goveetl automations set 42 'AA:BB:H706C' --power on --brightness 60 --temperature 2700
goveetl automations remove-device 42 'CC:DD:H616C'
goveetl automations show 42 --json
```

Writes remain experimental: live requests return Govee status 500. Report that
failure and the fresh saved state. Temperature edits support H706C at 2700 K only.
They replace saved scenes or colors. Power and brightness edits preserve the mode.
The CLI preserves other actions and the trigger, then verifies an accepted write.
Verification failure means the save remains unconfirmed; inspect it before retrying.
An automation read verifies stored settings, not a future trigger or visible light output.

Use `raw --backend app` when a private endpoint has no dedicated command.
This mode sends app identity headers and rejects Govee error statuses.
Keep raw responses out of logs: they can contain account topics and home coordinates.

## Limits and changes

Group scene mutations and token refresh remain unverified live. The APK endpoint
inventory does not imply complete CLI coverage. Consult the local evidence before
adding private endpoints, then verify the wire contract against the live service.

Add command routes and protocol tests in `internal/govee/`. Keep CLI wrappers thin.
Run `make check` before handoff. Use `goveetl-release` for merge and release work.
