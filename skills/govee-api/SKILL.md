---
name: govee-api
description: >-
  Drive Govee devices through goveetl, the repo's CLI. Use when the user asks
  about Govee lighting, plug/thermostat/humidifier control or state, or any
  goveetl subcommand: devices, control, groups, mqtt, lan, auth. Also covers
  extending goveetl (adding commands/endpoints) and debugging its API calls.
license: MIT
---

# Govee control via goveetl

This repo ships goveetl, a CLI for three Govee surfaces. Choose one per task:

| Surface | Auth | What it does | Verified |
| --- | --- | --- | --- |
| official | `Govee-API-Key` | device list with capabilities, state, control | live |
| app | app Bearer token | device list incl. models the official API omits, rooms/groups, scenes/DIY | client built; token capture pending |
| mqtt | app token + IoT cert | realtime per-device control and state push | built; needs login |
| lan | AES key from config | UDP discovery (port 4002) + control (port 4001) | built; needs hardware |

Prefer `official`; fall back to `app`/`mqtt` only for what the official API omits.

## Setup (once)

```bash
printf '%s' "$GOVEE_API_KEY" | goveetl config set api_key --stdin
goveetl doctor                 # reachability + config check
```

API key lives in 1Password item "Govee" (field `API_KEY`). Resolve at runtime:

```bash
export GOVEE_API_KEY="$(op item get Govee --field API_KEY)"
```

## Read device state

```bash
goveetl devices list                        # --backend api (default) or app
goveetl devices state "B4:11:D0:C9:07:BF:F5:60:H6006"
```

The `device:sku` reference splits on the LAST colon — device ids are MAC-style
with colons (e.g. `B4:11:D0:C9:07:BF:F5:60:H6006`).

## Send commands

```bash
goveetl control "<ref>" turn 1
goveetl control "<ref>" brightness 80
goveetl control "<ref>" color '{"r":255,"g":80,"b":0}'
goveetl control "<ref>" colorTemp 2700
goveetl control "<ref>" devices.capabilities.on_off/powerSwitch '{"v":1}'  # full form: <capabilityType>/<instance>
goveetl control "<ref>" turn 1 --capability-type devices.capabilities.on_off --instance powerSwitch  # explicit overrides
```

Route table (`internal/govee/openapi.go` `capabilityRoute`) maps short names to
`(capability type, instance)` per the official docs. Add entries there when a
device model exposes a new instance; never hard-code an untested route.

## Safe-change discipline

State reads support a check-then-act loop. Before mutating a device you have
not touched this session, read state first so you can restore it:

```bash
goveetl devices state "<ref>"
goveetl control "<ref>" brightness 80
goveetl control "<ref>" brightness 60   # restore
```

For anything visible to the household (Moment lights, Xmas trees), leave state
as you found it. Rate limits: control 12 rps burst 2 rps sustained per device;
state 30/min per device; devices list 30/min per account.

## Extending goveetl

Read `AGENTS.md` first — it is the repo contract for layout, tests, and
`make check` gates. Key rules while adding commands:

- put Govee wire types and request/response structs in `internal/govee/`
- commands live in `internal/cli/`, thin over the `govee` package
- add a capability-route entry + test in `govee_test.go` when adding a command name
- run `make check` before handoff

## Known limits

- `app` backend needs a one-time token capture (mitmproxy + Frida SSL unpinning
  on the Govee Home app); see `~/Documents/Govee_Goveetl/PROGRESS.md` Phase 5.
- Session login flow (email/password into the app API) sends encrypted bodies and
  is deliberately NOT implemented — credentials should never pass through a CLI.
- LAN commands are built but unverified against real hardware.
