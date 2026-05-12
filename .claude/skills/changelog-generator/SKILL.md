---
name: changelog-generator
description: Use when bootstrapping `CHANGELOG.md`, when cutting a release tag, or when authoring the body of a `gh release create`. Walks Conventional Commit history, categorizes by Features/Improvements/Fixes/Breaking/Security, emits release-ready Markdown. Adapted from skills.sh/composiohq/awesome-claude-skills/changelog-generator.
---

# changelog-generator (clyde-tuned)

clyde follows Conventional Commits per `CONTRIBUTING.md`. This skill turns that history into a `CHANGELOG.md` (Keep a Changelog format) and into release-note bodies for `gh release create`.

## Format (Keep a Changelog + clyde voice)

```markdown
# Changelog

All notable changes to clyde will be documented in this file. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- ...

### Changed
- ...

### Fixed
- ...

### Security
- ...

## [0.1.0] - 2026-MM-DD

Initial public release. Features: live multi-panel TUI for Claude Code sessions, hook-permission notifications via local HTTP server, Tokyo Night theme.

### Added
- live-session panel showing current tool call + mascot
- ...

[Unreleased]: https://github.com/Systemartis/clyde/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Systemartis/clyde/releases/tag/v0.1.0
```

## Bootstrap (one-time)

```bash
# 1. Get every commit since the project's initial-release commit
git log --oneline d5e916b..HEAD

# 2. Categorize by Conventional Commit type
git log --pretty=format:'%s' d5e916b..HEAD | awk -F: '
  /^feat/   { print "Added: "$0 }
  /^fix/    { print "Fixed: "$0 }
  /^refactor|^perf/ { print "Changed: "$0 }
  /^chore\(security|^fix\(security/ { print "Security: "$0 }
  /BREAKING/ { print "BREAKING: "$0 }
'

# 3. Hand-edit into the customer-grade voice. NOT raw git log.
```

The skill's "customer-grade voice" rule: rewrite developer prose into user-facing prose. Compare:

- ❌ Raw: `feat(tui): add livedata explorer adapter for git+fs Phase D`
- ✅ Customer: `Added a new "Explorer" panel showing live git status and the file tree of your project.`

## Per-release body (driven by `gh release create`)

```bash
# Tag first (Phase 2 of the production-readiness plan)
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0

# Build the body from the [0.1.0] section of CHANGELOG.md
gh release create v0.1.0 \
  --title "v0.1.0 — first public release" \
  --notes-file <(awk '/^## \[0.1.0\]/{f=1; next} /^## \[/{f=0} f' CHANGELOG.md) \
  ./dist/*.tar.gz \
  ./dist/checksums.txt
```

Note: with goreleaser, the release body comes from goreleaser's `changelog:` block which can be set to `use: github-native` OR `prepend: <(awk ... CHANGELOG.md)`. Pick one source of truth — recommended: hand-curated `CHANGELOG.md` is canonical, goreleaser's `changelog: { disable: true }` to skip auto-generation.

## What goes where

| Type | CHANGELOG section | Visible to user? |
|------|-------------------|------------------|
| `feat:` / `feat!:` | **Added** / **Changed** | yes |
| `fix:` | **Fixed** | yes |
| `perf:` | **Changed** (call out as "Performance") | yes |
| `refactor:` | usually omitted (no user impact) | no |
| `test:` | omitted | no |
| `docs:` | omitted unless docs were the user-visible deliverable | usually no |
| `chore:` / `chore(deps):` | omitted unless security-relevant | no |
| `chore(security):` / `fix(security):` | **Security** | yes |
| `ci:` | omitted | no |
| `BREAKING CHANGE` footer | top-of-section "**⚠ BREAKING:**" callout | yes — surface aggressively |

## Anti-patterns to avoid

- ❌ `git log --oneline` pasted as the changelog. Read like a robot, hide what matters.
- ❌ Emoji-only categories. Stick to Keep a Changelog headings.
- ❌ Multi-page changelogs in pre-1.0. Concise per-version sections; one paragraph + bullets.
- ❌ Including `chore(deps)` PRs. They're noise to users.

## Pre-cut checklist

Before tagging:

- [ ] `CHANGELOG.md` `[Unreleased]` section has at least one entry
- [ ] Move `[Unreleased]` content under a new dated `[X.Y.Z] - YYYY-MM-DD` heading
- [ ] Reset `[Unreleased]` to empty
- [ ] Update the comparison links at the bottom
- [ ] PR + merge the changelog update BEFORE tagging
- [ ] Tag the merge commit, not an arbitrary one

## Sources
- [composiohq/awesome-claude-skills/changelog-generator on skills.sh](https://skills.sh/composiohq/awesome-claude-skills/changelog-generator)
- [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
- [Conventional Commits](https://www.conventionalcommits.org/)
