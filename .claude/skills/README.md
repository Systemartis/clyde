# Project skills

This directory pins four skills that survived a master analysis of clyde (see `analysis/MASTER-ANALYSIS.md` for the ranking and rationale). Each one is sourced from skills.sh and re-tuned to match clyde's hexagonal layout and trust model.

| Skill | Use when | Source |
|-------|----------|--------|
| [`golang-pro`](./golang-pro/SKILL.md) | reviewing, writing, or refactoring any `.go` file | [skills.sh/jeffallan/claude-skills/golang-pro](https://skills.sh/jeffallan/claude-skills/golang-pro) |
| [`security-review`](./security-review/SKILL.md) | adding/changing secrets, hookserver, exec, HTTP, or user-controlled paths | [skills.sh/zackkorman/skills/security-review](https://skills.sh/zackkorman/skills/security-review) |
| [`improve-codebase-architecture`](./improve-codebase-architecture/SKILL.md) | refactoring, adding adapters/ports, or when a file passes ~500 LOC of branching | [skills.sh/mattpocock/skills/improve-codebase-architecture](https://skills.sh/mattpocock/skills/improve-codebase-architecture) |
| [`diagnose`](./diagnose/SKILL.md) | investigating any bug, flaky test, or unexpected behavior — *before* proposing a fix | [skills.sh/mattpocock/skills/diagnose](https://skills.sh/mattpocock/skills/diagnose) |

These are committed into the repo so every contributor (human or agent) sees the same checklist. They are **not** the upstream skills verbatim — each was rewritten to reference the actual files and patterns in clyde's tree (depguard rules, hookserver auth, JSONL dedup, git cache, etc.). Update them when the underlying code shape changes.

The full ranking, including skills that *didn't* make the cut and why, lives in [`../../analysis/MASTER-ANALYSIS.md`](../../analysis/MASTER-ANALYSIS.md).
