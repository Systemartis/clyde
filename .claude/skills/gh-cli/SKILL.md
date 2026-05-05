---
name: gh-cli
description: Use when cutting a release tag, uploading binaries, verifying signatures, or scripting any GitHub operation around clyde. Reference for `gh release create / upload / verify`, `gh repo edit`, `gh workflow run`, with clyde-specific recipes. Adapted from skills.sh/github/awesome-copilot/gh-cli.
---

# gh-cli (clyde-tuned)

Reference for the `gh` flows clyde uses. Pairs with `clyde-release-ritual` (which sequences the full launch checklist).

## Auth

```bash
gh auth login                         # one-time
gh auth status                        # verify
gh auth refresh -h github.com -s read:packages,write:packages   # only if publishing artifacts beyond Releases
```

## Release flow (executed by goreleaser, but useful for manual)

```bash
# Pre-release: ensure the tag exists
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0

# Create the release (goreleaser does this; manual fallback)
gh release create v0.1.0 \
  --title "v0.1.0 — first public release" \
  --notes-file <(awk '/^## \[0.1.0\]/{f=1; next} /^## \[/{f=0} f' CHANGELOG.md) \
  --draft

# Upload artifacts (goreleaser does this; manual fallback)
gh release upload v0.1.0 \
  ./dist/clyde_*_darwin_arm64.tar.gz \
  ./dist/clyde_*_darwin_amd64.tar.gz \
  ./dist/clyde_*_linux_arm64.tar.gz \
  ./dist/clyde_*_linux_amd64.tar.gz \
  ./dist/clyde_*_windows_amd64.zip \
  ./dist/checksums.txt \
  ./dist/sbom.cdx.json

# Promote draft to public
gh release edit v0.1.0 --draft=false

# Verify the release was published with the expected files
gh release view v0.1.0 --json assets --jq '.assets[].name'
```

## Verifying a downloaded binary

```bash
# Cosign keyless verification (Phase 4 of the plan ships this)
cosign verify-blob \
  --certificate ./clyde_v0.1.0_darwin_arm64.tar.gz.pem \
  --signature ./clyde_v0.1.0_darwin_arm64.tar.gz.sig \
  --certificate-identity-regexp "https://github.com/Systemartis/clyde/.+" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  ./clyde_v0.1.0_darwin_arm64.tar.gz

# SLSA provenance verification
slsa-verifier verify-artifact \
  --provenance-path provenance.intoto.jsonl \
  --source-uri github.com/Systemartis/clyde \
  ./clyde_v0.1.0_darwin_arm64.tar.gz
```

These invocations go in `SECURITY.md` as the user-facing verification recipe.

## Repo administration

```bash
# Enable Discussions
gh repo edit --enable-discussions

# Enable Issues (already on, but verify)
gh repo edit --enable-issues

# Set default branch (one-time)
gh repo edit --default-branch main

# Lock the repo to receive only PRs (no direct pushes — also requires branch protection)
gh api -X PUT "repos/Systemartis/clyde/branches/main/protection" \
  -f required_pull_request_reviews.required_approving_review_count=1 \
  -f restrictions=null \
  -f required_status_checks.strict=true \
  -f required_status_checks.checks[][context]=test \
  -f required_status_checks.checks[][context]=lint \
  -f required_status_checks.checks[][context]=fmt \
  -f required_status_checks.checks[][context]=vuln \
  -f enforce_admins=false
```

## Workflow operations

```bash
# Trigger a release workflow manually (rare — usually tag-driven)
gh workflow run release.yml --ref v0.1.0

# Watch a run
gh run watch

# List recent runs
gh run list --workflow=ci.yml --limit 5

# Re-run failed jobs
gh run rerun <run-id> --failed
```

## Issue / PR triage

```bash
# Quick inbox view
gh pr list --state open --json number,title,author,createdAt --limit 20

# Approve + merge after review
gh pr review <num> --approve
gh pr merge <num> --squash --delete-branch
```

## Scripting tip

`gh` supports `--jq` for JSON extraction. Useful in CI / runbooks:

```bash
LATEST_TAG=$(gh release list --limit 1 --json tagName --jq '.[0].tagName')
echo "Latest release: $LATEST_TAG"
```

## Sources
- [github/awesome-copilot/gh-cli on skills.sh](https://skills.sh/github/awesome-copilot/gh-cli)
- [`gh` manual](https://cli.github.com/manual/)
- [cosign verify-blob](https://docs.sigstore.dev/cosign/verifying/verify/)
- [slsa-verifier](https://github.com/slsa-framework/slsa-verifier)
