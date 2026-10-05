---
name: goveetl-release
description: >-
  Cut a goveetl release. Use when merging to main, tagging, or touching the
  release pipeline (.goreleaser.yaml, .github/workflows/release.yml), or when
  the Homebrew tap step misbehaves.
license: MIT
---

# goveetl release

## The one rule

Release only after `make check` is green on an **up-to-date `main`**. The
release workflow (`release.yml`) runs goreleaser on tag push `v*`, builds
six platform archives, uploads them as release assets, and updates the Homebrew
tap. Release only what you'd bet traffic on.

## Steps

1. Merge your feature PR into `main` (`gh pr merge --merge --admin`).
2. Tag from `origin/main`, not your local checkout:

   ```bash
   git tag -a vX.Y.Z -m "goveetl vX.Y.Z — <one-line summary>" origin/main
   git push origin vX.Y.Z
   ```

3. Watch until the run reaches a terminal state:

   ```bash
   gh run watch $(gh run list --limit 1 --json databaseId --jq '.[0].databaseId') --exit-status
   ```

4. Confirm the release carries **all 7 assets** — 6 archives + `checksums.txt`:

   ```bash
   gh release view vX.Y.Z --json assets,isDraft | python3 -c \
     "import json,sys; d=json.load(sys.stdin); print('draft:', d['isDraft'], '| assets:', len(d['assets']))"
   ```

5. Homebrew tap: goreleaser bumps `jwmoss/homebrew-tap` with the new version.
   Verify on the tap after a successful run.

## When the run fails

Read the failure before rerunning. Known failure modes:

- **Asset `already_exists` (HTTP 422)** — a prior attempt left release assets
  behind. Fix: `gh release delete vX.Y.Z --yes --cleanup-tag`, delete the local
  tag (`git tag -d vX.Y.Z`), re-tag from `origin/main`, push again. Deleting the
  release clears assets; re-tagging triggers a fresh workflow run.
- **`HOMEBREW_TAP_TOKEN` 401 / secret missing** — tap step aborts the whole run
  *after* assets upload. Set the token as a repo secret (fine-grained PAT with
  `repo` scope on `jwmoss/homebrew-tap`) before re-running.
- **Run rerun before deleting assets** — rerunning against existing assets is a
  no-op; the release step fails on name collision. Delete first.
- **Deprecated goreleaser config (`brews`)** — warning only; still leaves
  assets. Fix opportunistically, don't let it block a release.

## Discipline

Never hand-edit release notes or assets for a shipped tag. If the release is
wrong, delete it, redeploy from a clean tag. Roll forward via a new git tag;
don't stash fixes on the tag object itself.
