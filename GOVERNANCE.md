# Governance

Clyde is a small, focused project. Governance reflects that: lightweight, with explicit decision authority, and a path for that to change as the contributor base grows.

## Project ownership

Clyde is published by **Systemartis** (the AI consultancy in Bucharest, Romania). The Systemartis organization on GitHub holds the canonical repository, controls the release pipeline, and owns the trademark and any future commercial offerings derived from clyde.

## Maintainers

The current set of maintainers is listed in [MAINTAINERS.md](./MAINTAINERS.md). Maintainers have:

- **Merge authority** on the `main` branch.
- **Release authority** to cut tags that trigger the goreleaser workflow.
- **Issue triage authority** to close, label, and assign issues.

A maintainer can act unilaterally on routine work (bug fixes, dependency bumps, doc updates). Decisions that change architecture, public API, license, or governance require **two-maintainer agreement** — discussed in the relevant PR or issue, not in private channels.

## Adding maintainers

A contributor may be invited to join the maintainer list after a sustained track record of merged contributions and constructive review. There is no fixed bar. The existing maintainers vote (simple majority, vetoes from the project owner) and the new maintainer is added to MAINTAINERS.md and CODEOWNERS in a single PR.

A maintainer can step down at any time by opening a PR removing themselves from MAINTAINERS.md.

## Decision making

Most decisions are made in the open via GitHub issues and PRs. Where consensus isn't obvious:

1. **Technical disputes:** discuss in the PR. If unresolved after a few rounds, the project owner makes the call.
2. **Roadmap / scope:** maintainers propose, project owner decides.
3. **Security disclosures:** see [SECURITY.md](./SECURITY.md). The project owner coordinates the response.
4. **Code of Conduct violations:** see [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md). The project owner enforces; appeals route to the Systemartis org.

## License decisions

The project ships under [Apache 2.0](./LICENSE). Re-licensing or relicensing-with-exceptions requires unanimous maintainer agreement and explicit project-owner approval. We do **not** require contributors to sign a CLA — the Apache 2.0 inbound-equals-outbound clause is sufficient.

## Forks

Clyde is permissively licensed. You're welcome to fork, modify, and redistribute under the Apache 2.0 terms. The "Clyde" name and the visual identity are trademarks of Systemartis — see [README.md](./README.md) for the trademark notice.

## Changes to this document

Changes to GOVERNANCE.md require a PR and two-maintainer agreement. The project owner has veto authority.
