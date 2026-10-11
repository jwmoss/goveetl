#!/usr/bin/env python3
"""Read the configured live service through the compiled CLI."""

import json
from pathlib import Path
import subprocess
import sys

binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "bin/goveetl").resolve())


def read(*args):
    try:
        result = subprocess.run(
            [binary, "--no-input", "--timeout", "15s", "--json", *args],
            capture_output=True, text=True, timeout=60, check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        raise SystemExit("Live check failed: executable unavailable or deadline exceeded.") from None
    if result.returncode:
        raise SystemExit(f"Live check failed: CLI exit {result.returncode}. Check service access and saved credentials locally.")
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        raise SystemExit("Live check failed: command did not return JSON.") from None


def require(condition, message):
    if not condition:
        raise SystemExit("Live check failed: " + message)


devices = read("--dry-run", "devices", "list")
require(isinstance(devices, list) and len(devices) > 0, "account has no devices to inspect")
require(all(isinstance(d, dict) and d.get("device") and d.get("sku") for d in devices), "invalid device inventory")
selected = devices[0]
state = read("--dry-run", "devices", "state", selected["device"] + ":" + selected["sku"])
require(isinstance(state, dict) and state.get("code") == 200, "device state request was rejected")
payload = state.get("payload")
if payload is None:
    records = state.get("data")
    require(isinstance(records, list) and len(records) > 0, "missing device state")
    payload = records[0]
require(isinstance(payload, dict) and payload.get("device") == selected["device"], "state belongs to another device")
capabilities = payload.get("capabilities")
require(isinstance(capabilities, list) and any(isinstance(c, dict) and isinstance(c.get("state"), dict) and "value" in c["state"] for c in capabilities), "no device state values")
print(f"Live E2E passed: {len(devices)} devices; current state received for one device.")
