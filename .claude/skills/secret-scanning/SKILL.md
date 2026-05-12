---
name: secret-scanning
description: Use BEFORE flipping the clyde repo public, and on any PR that adds new code paths reading credentials. Activates GitHub Advanced Security secret scanning + push protection, configures alert exclusions, and codifies the rotate-first-then-scrub-history playbook. Adapted from skills.sh/github/awesome-copilot/secret-scanning.
---

# secret-scanning (clyde-tuned)

The clyde repo will flip public on `github.com/Systemartis/clyde`. Before that flip, every commit in history must be clean. After the flip, every PR must run through push protection.

## Pre-public sweep (one-shot)

Run this ONCE, before flipping the repo public:

```bash
# 1. History scan with gitleaks
brew install gitleaks
gitleaks detect --source . --report-path gitleaks.json --verbose

# 2. Custom patterns clyde should care about
cat > .gitleaks.toml <<'EOF'
[[rules]]
description = "Anthropic OAuth token"
regex = '''sk-ant-[A-Za-z0-9_-]{32,}'''
[[rules]]
description = "Anthropic API key"
regex = '''sk-ant-api[0-9]{2}-[A-Za-z0-9_-]{32,}'''
[[rules]]
description = "Hookserver token-shaped (256-bit hex)"
regex = '''[a-f0-9]{64}'''   # high false-positive rate; investigate hits
EOF
gitleaks detect --config .gitleaks.toml --source . --report-path gitleaks.json --verbose

# 3. Cross-check: any file ever named *.credentials.json or .credentials*
git log --all --diff-filter=A --name-only | grep -i credential

# 4. Test fixtures: confirm no real tokens leaked into testdata/
find . -path ./node_modules -prune -o -name "testdata" -print
# manually inspect each testdata/ directory
```

If any hit fires:
1. **Rotate first.** Anthropic OAuth → re-auth on a clean machine. Hookserver tokens are ephemeral; no rotation needed but understand the leak.
2. **Then scrub.** `git filter-repo` to remove the offending blob from history, force-push, notify any clones.
3. **Document the rotation** in `SECURITY.md` if it affected a real credential.

## Repo settings to enable (one-time, via repo Settings UI or `gh`)

```bash
# Enable Secret Protection (free for public repos)
gh secret-scanning alert list -R Systemartis/clyde   # confirms it's on

# Enable push protection (requires Advanced Security on private; free on public)
# Settings → Code security → Secret scanning → Push protection: Enabled
```

## `.github/secret_scanning.yml` (allowlist for known false positives)

```yaml
paths-ignore:
  # JSONL fixtures contain UUID-shaped strings that pattern-match as tokens
  - 'internal/adapters/jsonl/testdata/**'
  # processscan fixtures contain example session IDs
  - 'internal/adapters/processscan/testdata/**'
```

Keep `paths-ignore` minimal. Don't add a path to silence a hit unless you've verified the hit is a fixture, not a real secret.

## Per-PR push protection

GitHub blocks the push if a token-shaped string appears in the diff. Default behavior is correct; document for contributors:

> If push protection blocks your push:
> 1. Don't bypass. Inspect the diff.
> 2. If it's a real secret, rotate it. The credential is compromised the moment it hit your terminal history.
> 3. If it's a false positive (test fixture, example), add a path to `.github/secret_scanning.yml` and re-push.

## What clyde-specific secrets look like

- **Anthropic OAuth access token:** starts with `sk-ant-oat-`, ~64 chars. Lives in `~/.claude/.credentials.json`.
- **Anthropic OAuth refresh token:** same prefix, ~64 chars.
- **Hookserver per-process token:** 64 hex chars (32 bytes). Ephemeral, but never log or commit.
- **Anthropic API key (NOT used by clyde directly but adjacent):** `sk-ant-api03-...`.

Add custom regex patterns to `.gitleaks.toml` matching these shapes; never to `.github/secret_scanning.yml` (that's for path exclusions only).

## Sources
- [github/awesome-copilot/secret-scanning on skills.sh](https://skills.sh/github/awesome-copilot/secret-scanning)
- [GitHub docs: secret scanning](https://docs.github.com/en/code-security/secret-scanning)
- [gitleaks](https://github.com/gitleaks/gitleaks)
- [git-filter-repo](https://github.com/newren/git-filter-repo)
