---
name: supply-chain-risk-auditor
description: Use when adding a new direct dependency, when bumping a major version, or quarterly across the existing dep graph. Audits each dep against six risk dimensions (single maintainer, unmaintained, low popularity, FFI/deser, past CVEs, missing security contacts). Adapted from skills.sh/trailofbits/skills/supply-chain-risk-auditor for clyde's Go module graph.
---

# supply-chain-risk-auditor (clyde-tuned)

`govulncheck` catches **known** CVEs. This skill catches the **un**known: deps that look healthy today but have weak signals (single maintainer, no security policy, pseudo-version-only). Run quarterly across `go.mod`, and on every PR that adds a direct dep.

## The six risk dimensions

| Dimension | Signal | Action if positive |
|-----------|--------|--------------------|
| Single maintainer | `gh repo view <repo> --json owner,collaborators` shows one human commit author > 80% | document in `SUPPLY_CHAIN.md`; consider inlining if small |
| Unmaintained | last commit > 18 months on a dep with active downstream users | replacement plan; pin the last good version |
| Low popularity | <100 dependents on `pkg.go.dev` AND <500 GitHub stars | extra scrutiny on what the dep does inside clyde |
| High-risk features | uses `unsafe`, CGO, `encoding/gob` deser of untrusted data, runs subprocesses | special review; isolate behind a port |
| Past CVEs | `govulncheck` history or GitHub Security Advisories | accept only if patched + maintainer responded |
| Missing security contact | no `SECURITY.md`, no email in repo | escalate; this is the dimension we track for our own deps |

## clyde's current state (as of 2026-05-05)

From the wave-1 audit:

- **Single maintainer (acceptable, document):** `BurntSushi/toml`, `muesli/cancelreader`, `xo/terminfo`. Of these only `xo/terminfo` is concerning — pseudo-version, low activity.
- **Pseudo-version indirect deps (PR-04):** `charmbracelet/ultraviolet`, `charmbracelet/x/exp/golden`, `xo/terminfo`, `golang.org/x/exp`. Dependabot semver gating doesn't apply.
- **Non-canonical module path (PR-05):** Charm modules via `charm.land/*` redirect.
- **Heavy ecosystem concentration (PR-37):** 6 of 7 direct deps + 11 of 16 indirects are Charmbracelet.

## Workflow

```bash
# 1. Generate dep graph
go mod graph > dep-graph.txt
go list -m -u all > dep-versions.txt

# 2. For each direct dep, gather signals via gh CLI
for dep in $(go list -m -f '{{.Path}}' all | grep -v '^github.com/Systemartis/clyde'); do
  echo "## $dep"
  gh repo view "$dep" --json stargazerCount,pushedAt,licenseInfo,securityPolicyUrl 2>/dev/null
done > supply-chain-signals.md

# 3. Cross-reference with govulncheck
govulncheck -mode source ./...

# 4. Document findings in SUPPLY_CHAIN.md (one row per direct dep)
```

## SUPPLY_CHAIN.md template

```markdown
# Supply chain — direct dependencies

| Dep | Maintainer | Last activity | Stars | Security policy | Risk | Mitigation |
|-----|------------|---------------|-------|-----------------|------|------------|
| charm.land/bubbletea/v2 | charmbracelet org | <date> | <#> | yes/no | concentration (PR-37) | document; track Charm release cadence |
| github.com/BurntSushi/toml | single | <date> | <#> | no | bus factor | accept; small surface |
| ... |

## Pseudo-version indirects (PR-04)
- `charmbracelet/ultraviolet v0.0.0-20260416...` — coordinate with Charm to release a tagged version
- `xo/terminfo v0.0.0-20220910...` — replace with `golang.org/x/term` or `mattn/go-isatty`

## Charm concentration (PR-37)
clyde tracks Charmbracelet's release cadence. We accept the concentration because the alternative (multiple TUI libs) is worse.
```

## Triggers

- New direct dep (in PR review)
- Major version bump
- Quarterly cadence (Q1/Q2/Q3/Q4 first Monday)
- Before any tagged release

## Sources
- [trailofbits/skills/supply-chain-risk-auditor on skills.sh](https://skills.sh/trailofbits/skills/supply-chain-risk-auditor)
- [pkg.go.dev/cmd/govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
- [OSV.dev](https://osv.dev) for cross-ecosystem advisories
