---
name: clyde-release-ritual
description: Use when cutting any tagged release of clyde. Sequences the full ritual: version bump → CHANGELOG cut → tag → release workflow → smoke test → tap update → announcement. Custom skill — combines changelog-generator, gh-cli, and dependabot-tuning into one ordered checklist.
---

# clyde-release-ritual (custom)

The launch checklist as one skill. Run it for every tagged release. Skipping a step is allowed only with a documented reason in the release PR.

## Pre-flight (do these once per release window)

- [ ] **Master analysis findings closed?** F-01..F-09 from `analysis/MASTER-ANALYSIS.md` should be resolved before v0.1.0. After v0.1.0, just confirm none re-opened.
- [ ] **Production readiness Phase 1 closed?** PR-01 (`--version`), PR-03 (LICENSE), PR-07 (token URL), PR-14 (panic recovery) MUST be done before any tag.
- [ ] **Production readiness Phase 2 closed?** PR-02 (goreleaser config), PR-08 (CHANGELOG.md), PR-12 (semver doc), PR-09 (GOVERNANCE.md), PR-10/11 (badges + asciinema), PR-13 (trademark disclaimer).
- [ ] **CI green on `main`?** `gh run list --workflow=ci.yml --branch=main --limit 1`
- [ ] **`govulncheck` clean?** Last run output zero advisories.
- [ ] **`golangci-lint run` clean** with the F-02-fixed depguard rules.

## Cut the release

1. **Decide the version.** Use the `marcelorodrigo/conventional-commit` semver mapping:
   - `feat:` → MINOR (e.g., 0.1.0 → 0.2.0)
   - `fix:` → PATCH (0.1.0 → 0.1.1)
   - `BREAKING CHANGE:` or `feat!:` → MAJOR (0.1.0 → 1.0.0; pre-1.0, MINOR is fine)
2. **Update CHANGELOG.md.** Use the `changelog-generator` skill. Move `[Unreleased]` content under a new `[X.Y.Z] - YYYY-MM-DD` heading. Reset `[Unreleased]`. Update the comparison links at the bottom.
3. **Bump version metadata** if your release config requires it (e.g., `version` package constant — for clyde the plan uses `runtime/debug.ReadBuildInfo()` + ldflags, no constant to bump).
4. **PR + merge** the CHANGELOG + version bump. **Wait for CI green.**
5. **Tag the merge commit:**
   ```bash
   git checkout main && git pull
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
6. **Watch the release workflow:**
   ```bash
   gh run watch
   ```
7. **Verify the release page:**
   ```bash
   gh release view vX.Y.Z --json assets --jq '.assets[].name'
   ```
   Expected: 5 binaries (linux/amd64+arm64, darwin/amd64+arm64, windows/amd64), `checksums.txt`, `sbom.cdx.json`, `.sig` + `.pem` for each binary.

## Post-flight (Phase 4 onward — once signing/SBOM/tap are wired)

- [ ] **Smoke test on a fresh machine:**
  ```bash
  brew tap Systemartis/tap
  brew install clyde
  clyde --version
  # expected: vX.Y.Z (commit <SHA>) built <date>
  ```
- [ ] **Cosign verify** the binary you just installed (use the `gh-cli` skill's recipe).
- [ ] **SLSA verify** the provenance.
- [ ] **Update `analysis/MASTER-ANALYSIS.md` and `analysis/PRODUCTION-READINESS.md`** marking any closed findings.
- [ ] **Announce.** Discord, Twitter, blog post — whichever channels Systemartis maintains.

## Hot-fix path (security release)

If a `chore(security):` or `fix(security):` PR lands on `main`:

1. Skip the calendar; cut the release the same day.
2. CHANGELOG section gets a `### Security` heading.
3. Release notes link to a CVE / GHSA if one exists.
4. **Notify upstream Anthropic / Charm** if the issue affects their integration.
5. Update `SECURITY.md`'s past-advisories list.

## Anti-patterns

- ❌ Tagging a commit that hasn't been merged through PR + CI.
- ❌ Letting goreleaser auto-generate the changelog. We hand-curate `CHANGELOG.md`; goreleaser's `changelog: { disable: true }`.
- ❌ Skipping the `cosign verify-blob` smoke test "because the build worked." The whole point of signing is to validate it.
- ❌ Tagging on a Friday afternoon. Releases happen Monday-Thursday so issues land in business hours.

## Sources
- Skills used: `changelog-generator`, `gh-cli`, `dependabot-tuning`, `marcelorodrigo/conventional-commit` (spec only)
- Authoritative refs: `goreleaser.com`, `sigstore.dev`, `slsa.dev`
- Internal: `analysis/PRODUCTION-READINESS.md` §5 (phase ordering), `plans/2026-05-05-systemartis-launch.md`
