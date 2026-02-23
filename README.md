# tier

Agent-first CLI for managing stacked PRs with DAG dependencies on GitHub.

Written in Go. Single binary. Zero config. No accounts. No services.

## Why tier?

In the agentic coding era, developers and AI agents produce PRs at high velocity — often 10-20+ related PRs for a single feature. Existing stacking tools (Graphite, git-town, spr) are either too heavyweight, linear-only, TTY-dependent, or not designed for non-interactive agent use.

GitHub already handles single-parent branching and PR retargeting. What's missing is **dependency tracking between branches** — the DAG that tells agents and humans what's ready, what's blocked, and what order to merge.

tier adds exactly that. Nothing more.

## Install

```bash
go install github.com/nvandessel/tier@latest
```

Requires: [git](https://git-scm.com/), [gh](https://cli.github.com/) (GitHub CLI, authenticated)

## Quick start

```bash
# Create a feature branch (auto-detects trunk)
tier new feature/auth

# Create sub-branches
tier new auth/login --on feature/auth
tier new auth/signup --on feature/auth
tier new auth/e2e --on feature/auth --after auth/login,auth/signup

# Push and create PRs
tier push -t "Login flow"

# See the dependency graph
tier status

# After merging PRs on GitHub, sync everything
tier sync
```

## Commands

### `tier new <name> [--on <parent>] [--after <branch,...>]`

Create a new tracked branch and check it out.

- `--on` — Git parent (PR base). Default: current branch if tracked, else trunk.
- `--after` — Comma-separated logical dependencies. These branches must merge before this one.

```bash
tier new feature/payments --on main
tier new pay/api --on feature/payments --after pay/models,pay/db
```

### `tier push [-t <title>] [-b <body>] [--draft]`

Push current branch and create/update its GitHub PR.

- `-t` — PR title. Default: branch name humanized.
- `-b` — PR body.
- `--draft` — Create as draft PR.

Creates a new PR if none exists. If the PR's base branch has changed (e.g., after `tier sync` reparents), it retargets automatically.

### `tier sync`

Fetch, detect merged branches, clean up state, rebase unblocked branches.

1. Fetches from origin
2. Detects merged PRs via `gh`
3. Reparents children of merged branches, retargets their PRs
4. Removes merged branches from dependency lists
5. Rebases ready branches in topological order (skips blocked ones)

```
Synced:
  ✓ pay/stripe-client merged → removed
  ↑ pay/stripe-tests rebased onto feature/payments (was: pay/stripe-client)
  ↑ pay/api-handlers now unblocked [was blocked: pay/stripe-client]
  ● pay/e2e still blocked by: pay/api-handlers, pay/db-migrations
```

Exit code 2 on rebase conflicts. Resolve and run `tier sync` again.

### `tier status [--json] [--fetch]`

Show the dependency graph with readiness indicators.

- `--fetch` — Call `gh` to get live PR states (slower).
- `--json` — Structured JSON output.

```
main
├── feature/payments  #42
│   ├── pay/stripe-client  #43  [ready]
│   │   └── pay/stripe-tests  #44  [ready]
│   ├── pay/db-schema  #45  [ready]
│   │   └── pay/db-migrations  #46  [ready]
│   ├── pay/api-handlers  #47  [blocked: stripe-client, db-schema]
│   └── pay/e2e  (not pushed)  [blocked: api-handlers, stripe-tests, db-migrations]
└── feature/auth  #50
    └── auth/login  #51  [ready]
```

### `tier track <branch> --on <parent> [--after <branch,...>]`

Retroactively track an existing branch without checking it out.

### `tier untrack [<branch>]`

Remove a branch from tracking. Defaults to current branch. Reparents children and cleans dependency lists. Does not delete the git branch or its PR.

## Stacking patterns

### Wide (fan-out)

All sub-branches share the same git parent. Dependencies via `--after`.

```bash
tier new feature/auth          --on main
tier new auth/login            --on feature/auth
tier new auth/signup           --on feature/auth
tier new auth/password-reset   --on feature/auth --after auth/login
tier new auth/e2e              --on feature/auth --after auth/login,auth/signup,auth/password-reset
```

```
main
└── feature/auth                          PR → main
    ├── auth/login                        PR → feature/auth  [ready]
    ├── auth/signup                       PR → feature/auth  [ready]
    ├── auth/password-reset               PR → feature/auth  [blocked: auth/login]
    └── auth/e2e                          PR → feature/auth  [blocked: auth/login, auth/signup, auth/password-reset]
```

### Deep (linear stack)

Each branch stacks on the previous one via `--on`. Classic PR stacking.

```bash
tier new feature/api-v2    --on main
tier new api/models        --on feature/api-v2
tier new api/handlers      --on api/models
tier new api/middleware     --on api/handlers
```

When `api/models` merges, GitHub retargets `api/handlers` PR. `tier sync` detects this and updates state.

### Mixed (real-world)

Combine both. Deep stacks where there's sequential dependency, wide where work is parallel.

```bash
tier new feature/payments        --on main
tier new pay/stripe-client       --on feature/payments
tier new pay/stripe-tests        --on pay/stripe-client
tier new pay/db-schema           --on feature/payments
tier new pay/db-migrations       --on pay/db-schema
tier new pay/api-handlers        --on feature/payments  --after pay/stripe-client,pay/db-schema
tier new pay/e2e                 --on feature/payments  --after pay/api-handlers,pay/stripe-tests,pay/db-migrations
```

`--on` creates the git/PR hierarchy. `--after` creates the logical DAG. They're orthogonal.

## Agent workflow

tier is designed for non-interactive use. Every command supports `--json` for machine-readable output, never prompts for input, and uses clear exit codes.

```bash
# Agent creates a task branch
tier new auth/login --on feature/auth --json
# → {"name":"auth/login","parent":"feature/auth","after":[]}

# Agent pushes when done
tier push -t "Implement login flow" --json
# → {"branch":"auth/login","pr":51,"created":true}

# Agent checks what's ready
tier status --json
# → {"trunk":"main","branches":[...]}

# After reviews/merges, agent syncs
tier sync --json
# → {"merged":["auth/login"],"rebased":["auth/e2e"],...}
```

**Exit codes:** 0 = success, 1 = error, 2 = rebase conflict.

## Data model

State lives at `<git-common-dir>/tier.json` — shared across worktrees, invisible to the working tree.

```json
{
  "version": 1,
  "trunk": "main",
  "branches": {
    "auth/login": {
      "parent": "feature/auth",
      "after": [],
      "pr": 51
    }
  }
}
```

- `parent` — Git parent branch (PR base). Exactly one.
- `after` — Logical dependencies. Must all be merged before this branch can merge.
- `pr` — GitHub PR number. Null if not yet pushed.

## Design decisions

| Decision | Rationale |
|----------|-----------|
| Git tree + logical DAG | `parent` for PR base, `after` for merge ordering. Orthogonal concepts. |
| Shell out to `git` and `gh` | Thin wrapper. No go-git, no GitHub API client. Leverages user's existing auth. |
| No `tier init` | Lazy creation on first `tier new` or `tier track`. |
| No merge command | Merging is GitHub's job. `tier sync` handles the aftermath. |
| State in git-common-dir | Shared across worktrees. No .gitignore needed. |
| `--json` everywhere | Non-negotiable for agent consumption. |
| Lockfile for concurrency | Multiple agents in worktrees may modify tier.json simultaneously. |

## License

MIT
