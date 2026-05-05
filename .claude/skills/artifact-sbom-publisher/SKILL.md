---
name: artifact-sbom-publisher
description: Use when wiring SBOM generation, GitHub artifact attestations, or SLSA build provenance into the release workflow. Covers CycloneDX, Syft, SPDX, and the OIDC permissions block. Source — skills.sh/patricio0312rev/skills/artifact-sbom-publisher, clyde-tuned (Go-flavored).
---

# artifact-sbom-publisher (clyde-tuned)

Authoritative skill for closing PR-16 (SBOM + signing + attestations) from the production-readiness analysis. The upstream skill is Node-flavored; this rewrite swaps Node steps for Go and pins clyde's exact decisions.

## clyde's stack

| Concern | Tool | Why this one |
|---------|------|--------------|
| SBOM format | CycloneDX (default) + SPDX (fallback) | CycloneDX has better Go module fidelity; SPDX is the OSS license-compliance lingua franca |
| SBOM generator | `cyclonedx-gomod` for Go modules; `syft` for the binary archive | gomod is more accurate for `go.sum` resolution; syft picks up runtime libs in the archive |
| Signing | `cosign sign-blob --yes` (keyless, OIDC) | No keys to rotate. Sigstore Rekor transparency log gets us free auditability |
| Attestations | `actions/attest-build-provenance@v2` | First-party GitHub action; emits SLSA L3 provenance via OIDC |

## Workflow snippet (drop into `.github/workflows/release.yml`)

```yaml
permissions:
  id-token: write       # OIDC for cosign + attestations
  attestations: write   # GitHub artifact attestations
  contents: write       # release upload

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false   # PR-19

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          check-latest: true

      - name: Install cyclonedx-gomod
        run: go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1

      - name: Install syft
        run: |
          curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | \
            sh -s -- -b /usr/local/bin v1

      - name: Install cosign
        uses: sigstore/cosign-installer@v3

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Generate Go-module SBOM
        run: cyclonedx-gomod mod -licenses -json -output sbom.cdx.json

      - name: Sign each binary
        run: |
          for bin in dist/clyde_*/clyde dist/clyde_*/clyde.exe; do
            [ -f "$bin" ] || continue
            cosign sign-blob --yes --output-signature "${bin}.sig" --output-certificate "${bin}.pem" "$bin"
          done

      - name: Attest build provenance
        uses: actions/attest-build-provenance@v2
        with:
          subject-path: 'dist/**/clyde*'

      - name: Upload SBOM + sigs to release
        run: |
          gh release upload "${GITHUB_REF_NAME}" sbom.cdx.json dist/**/*.sig dist/**/*.pem
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Verification on a downloaded binary

This is what the `clyde-release-ritual` smoke test runs after the release publishes:

```bash
# 1. Verify cosign signature (keyless — Sigstore Rekor)
cosign verify-blob \
  --certificate clyde_linux_amd64.pem \
  --signature   clyde_linux_amd64.sig \
  --certificate-identity-regexp "https://github.com/Systemartis/clyde/.*" \
  --certificate-oidc-issuer     "https://token.actions.githubusercontent.com" \
  clyde_linux_amd64

# 2. Verify GitHub attestation (SLSA provenance)
gh attestation verify clyde_linux_amd64 --owner Systemartis

# 3. Inspect SBOM
jq '.components | length' sbom.cdx.json     # should match `go list -m all | wc -l` minus stdlib
jq '.components[].name' sbom.cdx.json | grep -c bubbletea   # spot-check known dep
```

## clyde-specific rules

- **Two SBOMs, not one.** `cyclonedx-gomod mod` covers the module graph (what was *intended*); `goreleaser`'s `sboms:` block (which uses syft under the hood) covers the archive (what *shipped*). Both are uploaded to the release. They will not match exactly — that's expected.
- **Keyless signing only.** No long-lived keys. `cosign sign-blob --yes` uses ambient OIDC from GHA. The `--yes` skips the interactive Sigstore notice (CI has no TTY).
- **Sign everything.** Every binary archive, plus `checksums.txt`. Skip per-file signatures inside the archive — verifying the archive sig is enough for downstream package managers.
- **`persist-credentials: false`** on every `actions/checkout` (PR-19). The release workflow doesn't need to push back; the brews tap PR is opened by goreleaser using a separate fine-grained PAT.
- **`subject-path: 'dist/**/clyde*'`** — globs every binary. attest-build-provenance recursively walks; we only ship one binary name per OS/arch so collisions aren't a concern.

## Common breakage

- **`cosign sign-blob` exits 1 with "no OIDC token"** — `id-token: write` permission is missing from the workflow. Add it at the job level (top-level is too coarse for security review).
- **Verification fails with "certificate identity does not match"** — the `--certificate-identity-regexp` must include the trigger ref, e.g., `tag/v0.1.0` not just the repo URL. Use the exact pattern Sigstore embeds.
- **SBOM has zero components** — cyclonedx-gomod was run before `go.sum` was populated. Run `go mod download` first, or use `goreleaser --clean` which does it implicitly.
- **`gh attestation verify` fails on macOS download** — sometimes Apple Gatekeeper quarantines the binary. `xattr -d com.apple.quarantine clyde` then retry.

## Sources

- Upstream skill: [skills.sh/patricio0312rev/skills/artifact-sbom-publisher](https://skills.sh/patricio0312rev/skills/artifact-sbom-publisher)
- Authoritative: [sigstore.dev](https://sigstore.dev), [slsa.dev](https://slsa.dev), [cyclonedx.org/specification/overview](https://cyclonedx.org), [actions/attest-build-provenance](https://github.com/actions/attest-build-provenance)
- Adjacent: [skills.sh/jim60105/copilot-prompt/add-artifact-attestations-to-workflow](https://skills.sh/jim60105/copilot-prompt/add-artifact-attestations-to-workflow) (Docker-image attestations — not used here, kept for reference if we ever ship a container image)
- Internal: `plans/2026-05-05-production-readiness.md` Phase 4, `clyde-release-ritual` skill
