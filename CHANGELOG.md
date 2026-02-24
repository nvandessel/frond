# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-02-23

First release. Minimal, agent-first CLI for managing stacked PRs with DAG dependencies on GitHub.

### Added

- `tier new` — create tracked branches with `--on` (parent) and `--after` (dependencies)
- `tier push` — push and create/update GitHub PRs with auto-generated titles
- `tier sync` — fetch, detect merged PRs, reparent children, rebase unblocked branches
- `tier status` — display dependency tree with readiness indicators and optional `--fetch` for live PR states
- `tier track` — retroactively track existing branches
- `tier untrack` — remove branches from tracking with automatic reparenting
- `tier completion` — shell completion scripts for bash, zsh, and fish
- `--json` output on every command for scripting
- DAG-based dependency tracking with cycle detection
- Lockfile with PID-based stale lock detection (cross-platform)
- Cross-platform support: Linux, macOS, Windows (amd64 + arm64)
- Homebrew tap via GoReleaser
- CI with GitHub Actions on Ubuntu and macOS

[0.1.0]: https://github.com/nvandessel/tier/releases/tag/v0.1.0
