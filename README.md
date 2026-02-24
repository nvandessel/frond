# frond

[![CI](https://github.com/nvandessel/frond/actions/workflows/ci.yml/badge.svg)](https://github.com/nvandessel/frond/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/nvandessel/frond/branch/main/graph/badge.svg)](https://codecov.io/gh/nvandessel/frond)
[![Go Report Card](https://goreportcard.com/badge/github.com/nvandessel/frond)](https://goreportcard.com/report/github.com/nvandessel/frond)
[![Release](https://img.shields.io/github/v/release/nvandessel/frond)](https://github.com/nvandessel/frond/releases/latest)

Minimal, agent-first CLI for managing stacked PRs with DAG dependencies on GitHub. 

Single binary. Zero config.

## Install

**Homebrew** (macOS/Linux):
```bash
brew install nvandessel/frond/frond
```

**Binary download**: grab the latest from [GitHub Releases](https://github.com/nvandessel/frond/releases/latest).

**From source**:
```bash
go install github.com/nvandessel/frond@latest
```

Requires [git](https://git-scm.com/) and [gh](https://cli.github.com/) (authenticated).

### Shell completions

```bash
frond completion bash > /etc/bash_completion.d/frond      # bash (Linux)
frond completion zsh > "${fpath[1]}/_frond"                # zsh
frond completion fish > ~/.config/fish/completions/frond.fish  # fish
```

## Usage

```bash
frond new feature/auth                                        # create tracked branch
frond new auth/login --on feature/auth                        # child branch
frond new auth/e2e --on feature/auth --after auth/login       # with dependency
frond push -t "Login flow"                                    # push + create PR
frond status                                                  # show dependency graph
frond sync                                                    # fetch, cleanup merged, rebase
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
| `frond new <name> [--on <parent>] [--after <deps>]` | Create tracked branch |
| `frond push [-t title] [-b body] [--draft]` | Push + create/update PR |
| `frond sync` | Fetch, detect merges, reparent, rebase |
| `frond status [--json] [--fetch]` | Show dependency graph |
| `frond track <branch> --on <parent> [--after <deps>]` | Track existing branch |
| `frond untrack [<branch>]` | Remove from tracking |

`--json` on every command. Exit codes: 0 success, 1 error, 2 conflict.

## Stacking patterns

`--on` creates the git/PR hierarchy (deep stacking). `--after` creates logical dependencies (wide fan-out). Combine both for real-world use:

```bash
frond new feature/payments        --on main
frond new pay/stripe-client       --on feature/payments
frond new pay/stripe-tests        --on pay/stripe-client                              # deep: stacks on stripe-client
frond new pay/db-schema           --on feature/payments
frond new pay/db-migrations       --on pay/db-schema                                  # deep: stacks on db-schema
frond new pay/api-handlers        --on feature/payments  --after pay/stripe-client,pay/db-schema    # wide: fan-out deps
frond new pay/e2e                 --on feature/payments  --after pay/api-handlers,pay/stripe-tests,pay/db-migrations
```

```
main
└── feature/payments                        PR → main
    ├── pay/stripe-client                   PR → feature/payments  [ready]
    │   └── pay/stripe-tests                PR → pay/stripe-client  [ready]
    ├── pay/db-schema                       PR → feature/payments  [ready]
    │   └── pay/db-migrations               PR → pay/db-schema  [ready]
    ├── pay/api-handlers                    PR → feature/payments  [blocked: stripe-client, db-schema]
    └── pay/e2e                             PR → feature/payments  [blocked: api-handlers, stripe-tests, db-migrations]
```

When `pay/stripe-client` merges, `frond sync` reparents `pay/stripe-tests`, unblocks `pay/api-handlers`, and rebases what's ready.

## Key concepts

- **`--on`** sets the git parent (PR base). One per branch.
- **`--after`** sets logical dependencies (merge ordering). Zero or more.
- These are orthogonal — `--on` for PR targeting, `--after` for merge ordering.
- State lives at `<git-common-dir>/frond.json` — shared across worktrees, invisible to the working tree.
