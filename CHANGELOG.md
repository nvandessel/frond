# Changelog

All notable changes to this project will be documented in this file.

## [0.3.2] - 2026-03-16

### Fixed

- **Stack comments scoped to connected branches** — PR comments now only show branches in the same stack, not every tracked branch (#67)
- **Comments applied for pre-existing PRs** — `frond track` and `frond push` now detect PRs created outside of frond and adopt them; `frond sync` always updates stack comments, not just after merges (#68)

### Added

- `ConnectedStack` in the `dag` package for filtering branches to a connected subtree
- `PRForBranch` in the `gh` package for detecting existing PRs by branch name

## [0.2.0] - 2026-02-27

### Added

- **Stack comments on PRs** — `frond push` and `frond sync` now post/update a bot-style comment on every PR in the stack showing the full dependency tree with the current PR highlighted (`👈`)
- `PRCommentList`, `PRCommentCreate`, `PRCommentUpdate` in the `gh` package for GitHub PR comment management
- `RenderStackComment` in the `dag` package for rendering highlighted stack trees in markdown
- Floop skill pack (`nvandessel/frond` v0.1.0) with 10 behaviors and 10 edges teaching AI agents stacked PR workflows

### Fixed

- Upgraded codecov action to v5 for tokenless uploads (#56)
- Restored `CODECOV_TOKEN` for protected branch uploads (#57)

### Changed

- GoReleaser config updated for v2 deprecations (#54, #55)
- Dependency bumps: `actions/checkout` v6, `actions/setup-go` v6, `goreleaser-action` v7

## [0.1.0] - 2026-02-23

First release. Minimal, agent-first CLI for managing stacked PRs with DAG dependencies on GitHub.

### Added

- `frond new` — create tracked branches with `--on` (parent) and `--after` (dependencies)
- `frond push` — push and create/update GitHub PRs with auto-generated titles
- `frond sync` — fetch, detect merged PRs, reparent children, rebase unblocked branches
- `frond status` — display dependency tree with readiness indicators and optional `--fetch` for live PR states
- `frond track` — retroactively track existing branches
- `frond untrack` — remove branches from tracking with automatic reparenting
- `frond completion` — shell completion scripts for bash, zsh, and fish
- `--json` output on every command for scripting
- DAG-based dependency tracking with cycle detection
- Lockfile with PID-based stale lock detection (cross-platform)
- Cross-platform support: Linux, macOS, Windows (amd64 + arm64)
- Homebrew tap via GoReleaser
- CI with GitHub Actions on Ubuntu and macOS

[0.3.2]: https://github.com/nvandessel/frond/releases/tag/v0.3.2
[0.2.0]: https://github.com/nvandessel/frond/releases/tag/v0.2.0
[0.1.0]: https://github.com/nvandessel/frond/releases/tag/v0.1.0
