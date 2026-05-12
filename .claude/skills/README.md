# Project skills

Pinned skills sourced from skills.sh and re-tuned to clyde's hexagonal layout, trust model, and OSS launch posture. Three analysis passes contributed:

- `analysis/MASTER-ANALYSIS.md` (correctness lens) — installed the first 4
- `analysis/PRODUCTION-READINESS.md` (OSS launch lens) — installed 7 more, plus 1 custom
- Wave-4 browser-driven re-sweep of skills.sh (the earlier WebFetch sweep had been fooled by the SPA's client-side filter and the leaderboard-only top page) — installed 3 additional skills that fill claimed gaps

| Skill | Use when | Source |
|-------|----------|--------|
| [`golang-pro`](./golang-pro/SKILL.md) | reviewing/writing any `.go` file | [skills.sh](https://skills.sh/jeffallan/claude-skills/golang-pro) |
| [`security-review`](./security-review/SKILL.md) | secrets / hookserver / exec / HTTP / user-paths | [skills.sh](https://skills.sh/zackkorman/skills/security-review) |
| [`improve-codebase-architecture`](./improve-codebase-architecture/SKILL.md) | refactor / new adapter / file >500 LOC of branches | [skills.sh](https://skills.sh/mattpocock/skills/improve-codebase-architecture) |
| [`diagnose`](./diagnose/SKILL.md) | investigating any bug — *before* proposing a fix | [skills.sh](https://skills.sh/mattpocock/skills/diagnose) |
| [`trailofbits-semgrep`](./trailofbits-semgrep/SKILL.md) | hookserver / OAuth / exec changes — run SAST | [skills.sh](https://skills.sh/trailofbits/skills/semgrep) |
| [`supply-chain-risk-auditor`](./supply-chain-risk-auditor/SKILL.md) | new direct dep / quarterly sweep / major bump | [skills.sh](https://skills.sh/trailofbits/skills/supply-chain-risk-auditor) |
| [`secret-scanning`](./secret-scanning/SKILL.md) | BEFORE going public; on any creds-touching PR | [skills.sh](https://skills.sh/github/awesome-copilot/secret-scanning) |
| [`dependabot-tuning`](./dependabot-tuning/SKILL.md) | reviewing `.github/dependabot.yml`; PR noise | [skills.sh](https://skills.sh/github/awesome-copilot/dependabot) |
| [`changelog-generator`](./changelog-generator/SKILL.md) | bootstrapping CHANGELOG; cutting a release | [skills.sh](https://skills.sh/composiohq/awesome-claude-skills/changelog-generator) |
| [`logging-best-practices`](./logging-best-practices/SKILL.md) | introducing `slog`; fixing TUI stderr corruption | [skills.sh](https://skills.sh/boristane/agent-skills/logging-best-practices) |
| [`gh-cli`](./gh-cli/SKILL.md) | release operations; verifying signatures | [skills.sh](https://skills.sh/github/awesome-copilot/gh-cli) |
| [`goreleaser`](./goreleaser/SKILL.md) | configuring `.goreleaser.yaml`, multi-arch, brew tap | [skills.sh](https://skills.sh/aaronflorey/agent-skills/goreleaser) |
| [`charmbracelet`](./charmbracelet/SKILL.md) | any TUI work — Bubble Tea / Lip Gloss / Bubbles | [skills.sh](https://skills.sh/aaronflorey/agent-skills/charmbracelet) |
| [`artifact-sbom-publisher`](./artifact-sbom-publisher/SKILL.md) | SBOM, cosign keyless signing, SLSA attestations | [skills.sh](https://skills.sh/patricio0312rev/skills/artifact-sbom-publisher) |
| [`clyde-release-ritual`](./clyde-release-ritual/SKILL.md) | every tagged release — sequences the full checklist | custom (Systemartis) |

These are committed into the repo so every contributor (human or agent) sees the same checklist. They are **not** the upstream skills verbatim — each was rewritten to reference the actual files and patterns in clyde's tree. Update them when the underlying code shape changes.

The full rankings — including skills that *didn't* make the cut and why, plus the wave-4 catalog re-verification — live in [`../../analysis/MASTER-ANALYSIS.md`](../../analysis/MASTER-ANALYSIS.md) §6 and [`../../analysis/PRODUCTION-READINESS.md`](../../analysis/PRODUCTION-READINESS.md) §3.
