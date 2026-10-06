# Changelog

## 1.1.0

- Add Govee Home automation lists and saved device-setting views without private messaging or location data.
- Add experimental automation light-setting edits and device removal with saved-state verification.
- Preserve automation list order, other device actions, and trigger settings during edits.
- Add `raw --backend app` with app identity headers and response-status checks.
- Return errors for Govee failures inside HTTP 200 responses.
- Extend dry-run protection to automation edits and raw app requests.
- Document the live status-500 limit for automation writes.

## 1.0.1

- Block cloud, app, MQTT, LAN, session, and config mutations during dry-run.
- Replace the incorrect LAN cipher and ports with the live plaintext protocol.
- Add LAN status and direct-address discovery.
- Correct the app host, app version header, and same-mode group membership.
- Correct MQTT device topics, message envelopes, TLS endpoints, and asynchronous errors.
- Add dynamic and DIY scene lists with the correct response envelope.
- Replace unsupported password login with explicit session-import guidance.
- Correct setup examples and document live verification limits.

## 1.0.0

- Release the initial CLI with live official API device lists, state, and brightness control.
