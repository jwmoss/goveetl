# Changelog

## 1.2.1

- Scope credentials and custom headers to the configured origin and guard redirects.
- Redact credential diagnostics and bound HTTP responses.
- Publish private config and session replacements without destination symlink traversal.
- Preserve exact raw JSON and correct config receipts, paths, and usage exit codes.
- Propagate provider timeouts and traces, and honor MQTT and LAN cancellation.
- Preserve official read POSTs, login verification, and dry-run mutation guards.
- Resolve module and VCS versions without config access.
- Update the MQTT dependency to fix reachable MQTT and proxy vulnerabilities.
- Use Go 1.27.1 and gate releases on native tests, race checks, and security scans.

## 1.2.0

- Add password login and email verification through the account REST API.
- Add hidden terminal prompts, stdin input, and script environment variables.
- Verify a new app session before saving it; preserve the existing session on failure.
- Add `auth status` and report authenticated access in human-readable doctor output.
- Reject empty refreshed tokens and keep credentials out of auth output.
- Restore mode 0600 before updating an existing configuration file.
- Document re-login as the verified renewal path and the remaining refresh rejection.

## 1.1.1

- Omit null object fields from automation writes to match the app's Gson serializer.
- Normalize nullable read metadata during saved-state verification.
- Fix warm-white automation saves that previously fail with Govee status 500.
- Verify a live H706C saved-setting update at 60% and 2700 K.

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
