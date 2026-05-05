# Install-Options Analysis

> **Goal:** Pick a one-command install path (or set of paths) that works for clyde's actual audience — terminal-native developers running Claude Code on macOS / Linux / Windows. Two flavours considered: **install the binary** ("install clyde on my machine") and **install as an AI-assistant skill** ("install clyde knowledge into my agent"). They are different problems with different answers.
>
> **Audience model:** clyde is launched alongside `claude` in a tile pane. Users already have Node, probably Homebrew (Mac) or a package manager preference (Linux), and possibly Go. They do *not* want to figure out their architecture or extract tarballs. The bar is `brew install`-grade convenience.
>
> **Method:** Surveyed 14 install patterns used by comparable Go/Rust TUIs (gh, lazygit, glow, k9s, fzf, bat, ripgrep, esbuild, swc, prisma, biome). Cross-referenced with [goreleaser's supported distributions](https://goreleaser.com/customization/) and [npm postinstall security patterns](https://github.com/npm/cli/issues/5234).
>
> **TL;DR recommendation:** Ship four channels — `brew` (primary Mac/Linux), `scoop` (Windows), `go install` (Go users), and a curl|sh installer at `install.systemartis.com/clyde` (everyone else, deeplink-friendly). Skip `npx`. Optionally publish a separate companion *skill* on skills.sh as `Systemartis/clyde-skill` so AI agents can teach themselves to launch clyde.

---

## 1. Audience-driven channel ranking

| Channel | Audience reached | Effort to ship | Effort to maintain | Risk | Recommendation |
|---------|------------------|----------------|--------------------|------|----------------|
| **Homebrew tap** (`brew install Systemartis/tap/clyde`) | macOS + Linux developers (~60% of clyde's audience) | M (already in goreleaser config) | L (tap auto-updates from goreleaser) | L | **Tier 1 — ship Phase 4** |
| **Scoop bucket** (`scoop install clyde` after `scoop bucket add systemartis https://github.com/Systemartis/scoop-bucket`) | Windows developers (~15% of audience) | S (goreleaser `scoops:` block) | L | L | **Tier 1 — ship Phase 4** |
| **`go install`** (`go install github.com/Systemartis/clyde/cmd/clyde@latest`) | Go developers (~30% overlap) | XS (works today; just document) | XS | M (no signing) | **Tier 2 — document** |
| **curl\|sh installer** (`curl -fsSL https://install.systemartis.com/clyde \| sh`) | Everyone with curl, copy-paste users | M (script hosting + cosign verify in script) | M (script must track release format) | M (security perception, real risk if domain is hijacked) | **Tier 2 — ship Phase 5** |
| **npx wrapper** (`npx @systemartis/clyde`) | Node developers, "I don't want to install anything" | M (postinstall script + per-arch packages) | H (5 platform packages, npm-publish-on-tag, supply-chain risk) | M-H | **Skip — not worth the maintenance burden** |
| **eget** (`eget Systemartis/clyde`) | Power users who already have eget | XS (works automatically with goreleaser archives) | XS | L | **Tier 3 — mention in README, don't promote** |
| **Manual download** (GitHub Releases) | Air-gapped users, package maintainers | XS (goreleaser produces these anyway) | XS | L | **Tier 3 — always available as fallback** |
| **Companion skill on skills.sh** (`npx skills add github.com/Systemartis/clyde-skill`) | AI agents that need to know clyde exists and how to launch it | S (separate skill repo) | S | L | **Tier 4 — optional, ship after launch** |
| nfpm (deb/rpm) | Linux distros, sysadmins, immutable distros | S (goreleaser `nfpms:` block) | M (need apt/yum repo hosting OR users manually install one-off `.deb`) | L | **Tier 4 — defer until requested** |
| Snap / Flatpak | Linux desktop users | M | M | M (sandbox breaks `~/.claude` paths) | **Skip — sandbox conflicts with clyde's read model** |
| AUR | Arch users | XS (community usually contributes) | XS (community-maintained) | L | **Tier 4 — accept community PR if offered** |
| Docker image | "Try without installing" | M | L | H (TUI in container needs `--tty -it -v ~/.claude:~/.claude`, breaks the simple-launch promise) | **Skip — defeats the purpose** |
| asdf / mise plugin | Multi-runtime devtool managers | M | M | L | **Skip — niche, no clear demand** |
| Cargo | Rust devs | n/a | n/a | n/a | **N/A — clyde is Go** |

---

## 2. Detailed pros & cons per channel

### 2.1 Homebrew tap — `brew install Systemartis/tap/clyde`

**Pros**
- The default expectation for any modern Mac CLI. macOS users will look here *first*.
- Works on Linux (linuxbrew). One channel, two operating systems.
- Auto-update via `brew upgrade`. Users self-serve patch releases.
- Goreleaser's `brews:` block auto-opens the tap PR; we never hand-write the formula.
- Trusted ecosystem — Homebrew has its own security review of formulae behavior; we benefit from that perception.
- We can include `assert_match version, shell_output("#{bin}/clyde --version")` in the formula's `test do` block — the tap update fails CI if the binary is broken.

**Cons**
- Requires a separate `Systemartis/tap` repo. Small overhead.
- Linuxbrew is less common than apt/yum/pacman — Linux server users won't reach for it.
- Formula updates lag a tagged release by ~30 seconds (PR + auto-merge). Acceptable.
- `brew` itself has supply-chain concerns (curl|bash itself, then runs Ruby). Not our problem to solve.

**Verdict:** Yes. Already wired in `plans/2026-05-05-systemartis-launch.md` Phase 4 and the `goreleaser` skill. Confirms as **Tier 1**.

---

### 2.2 Scoop bucket — `scoop install clyde`

**Pros**
- The Homebrew of Windows. PowerShell-native, no UAC prompt.
- Goreleaser's `scoops:` block auto-publishes the manifest to a bucket repo (`Systemartis/scoop-bucket`).
- Same maintenance model as the brew tap — set it up once, it auto-updates.
- Windows is a non-trivial slice of clyde's audience (devs running WSL or PowerShell + Claude Code).

**Cons**
- Scoop has lower adoption than Homebrew (~5M users vs 50M+). Many Windows devs use winget instead.
- Need a separate `Systemartis/scoop-bucket` repo.

**Verdict:** Yes — ship alongside brew. Add `winget` later if requested. **Tier 1**.

---

### 2.3 `go install github.com/Systemartis/clyde/cmd/clyde@latest`

**Pros**
- Works today; no infrastructure required.
- Idiomatic for Go developers — they expect this to work for any public Go repo.
- Pulls source, compiles locally — users can inspect what they're running.
- `runtime/debug.ReadBuildInfo()` populates `clyde --version` automatically (PR-01 covers this).

**Cons**
- Requires Go toolchain. Many target users don't have Go installed.
- Slow first run — cold compile of ~50 deps takes 10-30s. Subsequent runs are cached.
- No signing verification. The user trusts the Go module proxy (`proxy.golang.org`) and the `sum.golang.org` checksum DB.
- Long Go-version tail: users on Ubuntu LTS may have Go 1.21 when clyde requires Go 1.26. `go install` errors with `unknown directive`.
- No managed update path — `go install ...@latest` re-runs each time, but no auto-update.

**Verdict:** Document it, don't promote it. **Tier 2.** Always works for the Go-native audience; not the right default for everyone else.

---

### 2.4 curl|sh installer — `curl -fsSL https://install.systemartis.com/clyde | sh`

**Pros**
- The lowest-friction path for anyone with curl. One copy-paste line.
- Works on every Unix, including WSL and CI.
- Can detect OS/arch and grab the right tarball from GitHub Releases.
- Can verify the cosign signature *inside the script* before extraction — actually safer than letting the user blindly trust a tarball.
- We control the script — we can update it without users reinstalling.
- Pattern is well-established: Bun, Deno, Astral (uv), Sentry, fly.io, Cloudflare wrangler all use this.

**Cons**
- Long-standing security debate ("never pipe curl to sh"). Some users will refuse on principle. Mitigation: the script is short (<100 lines), readable at the URL, and we recommend `curl -fsSL <url> -o /tmp/install-clyde.sh && less /tmp/install-clyde.sh && sh /tmp/install-clyde.sh` for the paranoid.
- Requires hosting infra. Options:
  - GitHub raw URL (`raw.githubusercontent.com/Systemartis/clyde/main/install.sh`) — free, but ugly URL.
  - Custom subdomain (`install.systemartis.com`) — branded, requires Cloudflare/Caddy redirect to GitHub raw.
  - Vercel/Netlify static hosting — overkill.
- A stolen domain or compromised CDN poisons every future install. Mitigation: pin a specific commit SHA in the URL for security-sensitive users (`/clyde/v1.0.0/install.sh`), and document the cosign verify step.
- Script must track release-asset naming exactly. If goreleaser changes archive layout, the script breaks.

**Verdict:** Yes — ship as **Tier 2** in Phase 5. The branded subdomain (`install.systemartis.com`) is worth the small DNS overhead. Always sign-verify within the script.

**Reference implementations to copy from:**
- [`bun.sh/install`](https://bun.sh/install) — clean, robust, includes verification
- [`astral.sh/uv/install.sh`](https://astral.sh/uv/install.sh) — Rust-binary install, includes cosign verify
- [`deno.land/install.sh`](https://deno.land/x/install/install.sh) — same pattern

---

### 2.5 npx wrapper — `npx @systemartis/clyde`

**Pros**
- Familiar to JS developers. `npx` is preinstalled with Node.
- "Install" is implicit — npx caches and runs without a permanent install.
- Cross-platform "for free" — npm runs anywhere Node runs.
- High discoverability — `npm search clyde` works.

**Cons**
- Requires Node.js. clyde has no other Node dependency. We'd be telling users "install a JS runtime to run our Go binary." Frustrating.
- Implementation pattern: a parent package (`@systemartis/clyde`) with a `postinstall` script that downloads the right binary, plus optional per-arch packages (`@systemartis/clyde-darwin-arm64`, etc.) that ship binaries via `optionalDependencies` (esbuild's pattern). This is **5+ packages to maintain** per release.
- npm postinstall scripts have a poor security reputation — npm itself recently disabled them by default in some configurations. Users may have to `--ignore-scripts=false`, which they won't.
- Supply-chain risk: an npm token compromise lets an attacker push a malicious binary to anyone running `npx @systemartis/clyde`. Same risk class as a brew tap compromise, but npm's history of token leaks is worse.
- Slow first run — npm fetches the package + binary + extracts. Cached subsequent runs are fast.
- Some node versions (especially in restricted CI environments) don't allow downloading external binaries during `npm install`. Breaks in Docker images that disable network at install.

**Verdict:** **Skip.** The maintenance is 5× the brew tap cost for a worse experience. The Node-native audience can use `go install` or curl|sh just as easily. Revisit if a clear demand emerges (e.g., 100+ thumbs on a "please publish to npm" issue).

**Counter-evidence:** swc, esbuild, biome ship via npm because their *primary* audience is JS toolchain users. clyde's primary audience is terminal-native devs running Claude Code — npm-fluent but not Node-pinned. Different math.

---

### 2.6 eget — `eget Systemartis/clyde`

**Pros**
- Generic GitHub-release installer. Works automatically once goreleaser uploads archives with predictable naming.
- Verifies SHA256 from `checksums.txt` — bit of free supply-chain hygiene.
- Zero work for us beyond clean release-asset naming.

**Cons**
- `eget` itself has a chicken-and-egg problem. Users who don't already have it won't install eget just to install clyde.
- Niche — power-user tool, not mainstream.

**Verdict:** Mention in the README's "alternative installs" section. No work needed. **Tier 3.**

---

### 2.7 GitHub Releases manual download

**Pros**
- Always available via goreleaser. No extra work.
- The fallback when everything else breaks.
- Air-gapped / regulated environments need this.

**Cons**
- Not "one command." Users must figure out their arch, extract the tarball, chmod, place on PATH.
- Power users only. Many people don't know what `~/.local/bin` is.

**Verdict:** Always ship. Document in README's install section as "manual install." **Tier 3.**

---

### 2.8 Companion skill on skills.sh — `npx skills add github.com/Systemartis/clyde-skill`

This is the *other* meaning of "install clyde for an AI assistant." Different concern from installing the binary.

**Pros**
- AI agents (Claude Code, Cursor, Codex) can self-discover that clyde exists and learn how to launch it.
- Free distribution channel — skills.sh has a leaderboard and search.
- Reinforces Systemartis brand within the agent ecosystem.
- The skill content is small: one SKILL.md explaining "to inspect Claude Code sessions, install and run `clyde` (see install commands below)."

**Cons**
- Separate repo to maintain (skills.sh skills must be a public GitHub repo).
- Adds another release artifact to the ritual.
- The skill is meta — it teaches an agent to teach a user to install clyde. Adds a step.

**Verdict:** **Tier 4 — optional, ship after launch.** Worth doing once we have ≥ 1.0.0 stable. Not launch-blocking. The skill's natural home is `github.com/Systemartis/clyde/.claude/skills/clyde/` (already mostly written — we just need to publish).

---

### 2.9 nfpm packages (deb / rpm)

**Pros**
- Native package managers on Debian/Ubuntu/Fedora. First-class for sysadmins.
- Goreleaser produces them automatically with the `nfpms:` block.
- Can be signed with GPG.

**Cons**
- To get auto-update (`apt upgrade`), we'd need to host an apt repo. Significant infra: signed `Release` files, `Packages` index, GPG key distribution.
- Users can `dpkg -i clyde_*.deb` manually, but that's not "auto-update" — same as a tarball with extra steps.
- Packagecloud / Cloudsmith / GitHub apt-style hosting all charge or have limits.

**Verdict:** **Tier 4 — defer.** Generate the `.deb`/`.rpm` files via goreleaser and upload to GitHub Releases (zero hosting cost). Don't run an apt repo until there's clear demand.

---

### 2.10 Snap / Flatpak

**Verdict:** **Skip.** clyde reads `~/.claude/projects/...` — Snap and Flatpak sandboxes break this without user-granted full-home permission. The UX of "install clyde, now grant home access" is worse than tarball install.

---

### 2.11 Docker image — `docker run systemartis/clyde`

**Verdict:** **Skip.** clyde needs a real TTY, the user's `~/.claude` directory, the user's git working tree, and a hookserver listening on localhost. To run usefully in Docker requires `--tty -it -v ~/.claude:/root/.claude -v $PWD:/work --network=host`. At that point, "install Docker, then run a long flag string" is worse than installing the binary. Not the right tool.

---

### 2.12 winget (Windows Package Manager)

**Pros**
- The "official" Microsoft package manager for Windows. Microsoft is investing heavily.
- Probably more relevant than Scoop on a 3-year horizon.

**Cons**
- Goreleaser doesn't natively publish to winget (as of writing). Need a separate `.github/workflows/winget.yml` using `microsoft/winget-create-action` after the release lands.
- Manifest review is human — submissions take days for first-time publishers, hours after that.

**Verdict:** **Tier 4.** Add after Scoop is stable. Not launch-blocking.

---

## 3. Recommended channel mix for v0.1.0

```
                    Tier 1                Tier 2              Tier 3
              ┌──────────────────┐  ┌─────────────────┐  ┌──────────────┐
macOS         │ brew install     │  │ go install      │  │ manual GitHub│
              │ Systemartis/tap  │  │ ...             │  │ Releases     │
Linux         │ brew install ... │  │ curl install.sh │  │ eget         │
                                 │  │ go install ...  │  │              │
Windows       │ scoop install    │  │ go install ...  │  │ manual       │
              └──────────────────┘  └─────────────────┘  └──────────────┘
```

**Two new infrastructure pieces required to ship Tier 1+2:**

1. **`Systemartis/tap`** repo (Homebrew tap) — one-time setup, goreleaser auto-pushes formulae.
2. **`Systemartis/scoop-bucket`** repo (Scoop bucket) — same pattern.
3. **`install.systemartis.com/clyde`** — DNS + Cloudflare redirect to `raw.githubusercontent.com/Systemartis/clyde/main/install.sh` (or pinned tag for production). Optional shorter alternate: `install.clyde.sh` if we want a project-specific subdomain.

**Plan landing point:** This becomes Phase 8 of the consolidated launch plan: **"Install Capability."** Sequence after Phase 4 (signing/SBOM/tap is wired) but before Phase 6 (announce).

## 4. Companion skill (optional, after launch)

If we want clyde to be discoverable by AI assistants (the second meaning of "install"), publish a separate one-file skill repo:

```
github.com/Systemartis/clyde-skill/
└── SKILL.md          # tells the agent: "to use clyde, run `brew install Systemartis/tap/clyde` then run `clyde` next to claude"
```

Then publish to skills.sh (`npx skills add github.com/Systemartis/clyde-skill`). Free agent-side discovery. Defer to post-1.0.0.

## 5. What I considered and rejected

- **A npm `@systemartis/clyde` wrapper.** Maintenance / supply-chain cost > benefit. Node devs can use `curl|sh` or `go install`.
- **A self-hosted apt/yum repo.** Hosting, signing, and CI complexity for marginal user gain over a `.deb` on the release page.
- **Snap / Flatpak.** Sandbox breaks `~/.claude` reads.
- **Docker image.** TTY + volume mounts make "one command" longer than installing the binary.
- **A custom installer that auto-updates** (à la `cargo install-update`). Reinventing what brew already does. Not worth.

## 6. Open question for the user

> Do you want to ship `install.systemartis.com/clyde` (branded subdomain) or `install.clyde.sh` (project subdomain)?

The branded one reinforces Systemartis. The project one is shorter and doesn't lock clyde to the parent org — easier if clyde ever spins out. **Recommendation: `install.systemartis.com/clyde`** — clyde is a Systemartis flagship, not a future spinoff candidate.

## Sources

- Comparable Go TUIs: [gh](https://github.com/cli/cli), [glow](https://github.com/charmbracelet/glow), [k9s](https://k9scli.io/), [lazygit](https://github.com/jesseduffield/lazygit)
- npm wrapper pattern: [esbuild's npm package](https://github.com/evanw/esbuild/blob/main/lib/npm), [biome's distribution](https://github.com/biomejs/biome/tree/main/packages)
- Curl|sh exemplars: [bun.sh/install](https://bun.sh/install), [astral.sh/uv](https://astral.sh/uv/install.sh)
- Goreleaser distribution targets: [goreleaser.com/customization](https://goreleaser.com/customization/)
- Scoop publishing: [goreleaser.com/customization/scoop](https://goreleaser.com/customization/scoop/)
- Companion skill pattern: skills.sh ecosystem
