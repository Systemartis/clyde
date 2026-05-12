---
name: dependabot-tuning
description: Use when reviewing the existing `.github/dependabot.yml`, when adding a new package ecosystem, or when PR noise from updates becomes overwhelming. Tunes grouping, scheduling, and security-update policies for clyde's Go module + GitHub Actions surface. Adapted from skills.sh/github/awesome-copilot/dependabot.
---

# dependabot-tuning (clyde-tuned)

clyde already has `.github/dependabot.yml` covering `gomod` + `github-actions`. This skill captures the tuning we'd apply once the project is public and PR volume goes up.

## Recommended `.github/dependabot.yml`

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
      day: monday
      time: "06:00"
      timezone: Europe/Bucharest
    open-pull-requests-limit: 5
    labels: ["dependencies", "go"]
    commit-message:
      prefix: chore(deps)
      include: scope
    groups:
      charm:
        patterns:
          - "charm.land/*"
          - "github.com/charmbracelet/*"
        update-types: ["minor", "patch"]
      golang-x:
        patterns:
          - "golang.org/x/*"
        update-types: ["minor", "patch"]
      # security updates always land as individual PRs (override group)
    # Allow groups to be overridden by security-only updates
    allow:
      - dependency-type: direct
      - dependency-type: indirect

  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
      day: monday
    open-pull-requests-limit: 3
    labels: ["dependencies", "ci"]
    commit-message:
      prefix: chore(deps)
      include: scope
    groups:
      actions:
        patterns:
          - "actions/*"
          - "golangci/*"
        update-types: ["minor", "patch"]
```

## Why these knobs

- **`groups:` for Charm** — the Charmbracelet ecosystem releases lockstep across `bubbles`/`bubbletea`/`lipgloss`. Grouping them into one PR per week saves 3 reviews per release cycle. Major bumps still split out.
- **`groups:` for `golang.org/x/*`** — same reasoning; `x/sys`, `x/sync`, `x/term` move together.
- **`groups:` for actions/* + golangci/* in CI** — Actions updates are mechanically reviewed (verify SHA pin, verify nothing broke); grouping them is safe.
- **Per-week cadence** — daily would be noisy; monthly would back up. Monday morning lets the maintainer batch.
- **Conventional commit prefix** — feeds `composiohq/changelog-generator` cleanly.

## Security updates

Dependabot auto-opens security update PRs **outside** the group structure when a CVE drops. Don't disable this. Review and merge within 48h per the SLA we'll add to `SECURITY.md`.

## What to NOT do

- Don't add `package-ecosystem: docker` unless clyde grows a Dockerfile. (It hasn't; verified.)
- Don't add `npm` / `pip` / `cargo`. clyde is Go-only.
- Don't disable `update-types: indirect`. Indirect deps with pseudo-versions (PR-04) ARE the supply-chain risk.

## After this lands

Verify the next dependabot run produces grouped PRs:

```bash
gh pr list --label dependencies --json title,createdAt --limit 10
```

If you see > 1 PR for `charm.land/*` in the same week, the grouping isn't taking. Common cause: pattern syntax (use `charm.land/*` not `charm.land/**`).

## Sources
- [github/awesome-copilot/dependabot on skills.sh](https://skills.sh/github/awesome-copilot/dependabot)
- [GitHub docs: dependabot.yml reference](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference)
