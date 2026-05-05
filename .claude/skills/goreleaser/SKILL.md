---
name: goreleaser
description: Use when configuring `.goreleaser.yaml`, wiring the release workflow, or troubleshooting a tagged build. Covers multi-arch builds, ldflags injection, Homebrew tap, signing, SBOM, and changelog. Source — skills.sh/aaronflorey/agent-skills/goreleaser, clyde-tuned.
---

# goreleaser (clyde-tuned)

Authoritative skill for clyde's release pipeline. The upstream skill ships reference files for `builds`, `archives`, `docker`, `nfpm`, `homebrew`, `signing`, `changelog`, `ci`, `templates`, and `examples` — read it directly when wiring a phase. This file pins the clyde-specific decisions so we don't re-litigate them every release.

## clyde's `.goreleaser.yaml` baseline

Per `plans/2026-05-05-production-readiness.md` Phase 2, the config we ship is:

```yaml
# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
version: 2
project_name: clyde

builds:
  - main: ./cmd/clyde
    binary: clyde
    env: [CGO_ENABLED=0]
    flags: [-trimpath, -buildvcs=true]
    ldflags:
      - -s -w
      - -X github.com/Systemartis/clyde/internal/version.version={{.Version}}
      - -X github.com/Systemartis/clyde/internal/version.commit={{.Commit}}
      - -X github.com/Systemartis/clyde/internal/version.date={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]

sboms:
  - artifacts: archive

# We hand-curate CHANGELOG.md — never let goreleaser generate it.
changelog:
  disable: true

brews:
  - repository:
      owner: Systemartis
      name: tap
    homepage: https://github.com/Systemartis/clyde
    description: Terminal companion TUI for Claude Code
    license: MIT
    test: |
      assert_match version, shell_output("#{bin}/clyde --version")
```

## clyde-specific rules

- **`CGO_ENABLED=0` is non-negotiable.** clyde must run on bare Alpine / scratch images. Re-introducing CGO would break `go install`-from-source users on the long tail of distros.
- **`-trimpath` and `-buildvcs=true` together.** `-trimpath` strips the build machine's home dir from binary paths (privacy, reproducibility). `-buildvcs` embeds VCS metadata that `runtime/debug.ReadBuildInfo()` can read at runtime — this is what `version.Info()` falls back on when ldflags are empty (e.g., `go install`).
- **Version package, not `var version` in main.** clyde's version sentinel lives at `internal/version` (per PR-01). ldflags inject into `internal/version`, not `main`.
- **Hand-curated `CHANGELOG.md`.** `changelog.disable: true`. We use the `changelog-generator` skill against Conventional Commits, then a human edits before tagging. Goreleaser's auto-changelog is too noisy for a public-facing changelog.
- **SBOM as archive sibling, not separate target.** `sboms: [{ artifacts: archive }]` is enough — produces one `.sbom.cdx.json` per archive. Don't enable `sboms.artifacts: source` until we have a story for source-tree SBOMs.
- **Homebrew tap is `Systemartis/tap`** (not a per-project tap). Same tap will host other Systemartis CLIs as they ship.

## Pre-tag verification

```bash
goreleaser check                          # validate config
goreleaser release --snapshot --clean     # full build, no publish
ls dist/                                  # 5 binaries × archive + checksums + SBOM
file dist/clyde_linux_amd64_v1/clyde      # confirm static
```

If `--snapshot` succeeds locally, the tag-triggered release will too — assuming the workflow has the right secrets (cosign OIDC, brew token).

## Common breakage

- **`{{.Version}}` is empty in snapshot mode** — that's expected. The snapshot uses `0.0.0-next` or similar. Don't add `if .IsSnapshot` guards; the production tag fixes it.
- **`brews:` push fails** — usually the tap repo doesn't exist yet, or the token lacks `contents: write`. Create `Systemartis/tap` first; provision a fine-grained PAT scoped to that repo only.
- **Multi-arch Docker** is intentionally NOT configured. clyde is a TUI; a container image makes no sense. If we ever need one, use `dockers_v2:` (not legacy `dockers:`).

## Sources

- Upstream skill: [skills.sh/aaronflorey/agent-skills/goreleaser](https://skills.sh/aaronflorey/agent-skills/goreleaser)
- Authoritative: [goreleaser.com](https://goreleaser.com)
- Internal: `plans/2026-05-05-production-readiness.md` Phase 2, `clyde-release-ritual` skill
