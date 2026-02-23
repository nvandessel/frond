# tier

[![CI](https://github.com/nvandessel/tier/actions/workflows/ci.yml/badge.svg)](https://github.com/nvandessel/tier/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/nvandessel/tier/branch/main/graph/badge.svg)](https://codecov.io/gh/nvandessel/tier)
[![Go Report Card](https://goreportcard.com/badge/github.com/nvandessel/tier)](https://goreportcard.com/report/github.com/nvandessel/tier)
[![Release](https://img.shields.io/github/v/release/nvandessel/tier)](https://github.com/nvandessel/tier/releases/latest)

Minimal, agent-first CLI for managing stacked PRs with DAG dependencies on GitHub. Single binary. Zero config.

## Install

```bash
go install github.com/nvandessel/tier@latest
```

Requires [git](https://git-scm.com/) and [gh](https://cli.github.com/) (authenticated).

## Usage

```bash
tier new feature/auth                                        # create tracked branch
tier new auth/login --on feature/auth                        # child branch
tier new auth/e2e --on feature/auth --after auth/login       # with dependency
tier push -t "Login flow"                                    # push + create PR
tier status                                                  # show dependency graph
tier sync                                                    # fetch, cleanup merged, rebase
```

```
main
├── feature/auth  #42
│   ├── auth/login  #43  [ready]
│   ├── auth/signup  #44  [ready]
│   └── auth/e2e  (not pushed)  [blocked: auth/login, auth/signup]
```

## Commands

| Command | Description |
|---------|-------------|
| `tier new <name> [--on <parent>] [--after <deps>]` | Create tracked branch |
| `tier push [-t title] [-b body] [--draft]` | Push + create/update PR |
| `tier sync` | Fetch, detect merges, reparent, rebase |
| `tier status [--json] [--fetch]` | Show dependency graph |
| `tier track <branch> --on <parent> [--after <deps>]` | Track existing branch |
| `tier untrack [<branch>]` | Remove from tracking |

`--json` on every command. Exit codes: 0 success, 1 error, 2 conflict.

## Key concepts

- **`--on`** sets the git parent (PR base). One per branch.
- **`--after`** sets logical dependencies (merge ordering). Zero or more.
- These are orthogonal: `--on` creates the PR hierarchy, `--after` creates the DAG.
- State lives at `<git-common-dir>/tier.json` — shared across worktrees, invisible to the working tree.

## License

Apache 2.0
