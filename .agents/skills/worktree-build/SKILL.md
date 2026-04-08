---
name: worktree-build
description: >-
  Build the RWX CLI inside a git worktree. The LSP bundle is gitignored and
  won't be present in new worktrees, so it must be copied from the main repo
  before `go build` will succeed. TRIGGER when: a `go build`, `go vet`, or
  similar command fails with "pattern bundle/server.js: no matching files found"
  inside a worktree.
---

# Building in a Git Worktree

The `internal/lsp/embed.go` file uses `//go:embed` to bundle `bundle/server.js`.
That file is produced by `make build` (which clones and compiles the language-server
repo) and is gitignored, so it is **not** present in fresh worktrees.

## Fix

Copy `server.js` from the main repo into the worktree:

```bash
# Identify the main repo root (parent of .claude/worktrees/)
MAIN_REPO="$(git worktree list --porcelain | head -1 | sed 's/worktree //')"
WORKTREE_ROOT="$(git rev-parse --show-toplevel)"

mkdir -p "$WORKTREE_ROOT/internal/lsp/bundle"
cp "$MAIN_REPO/internal/lsp/bundle/server.js" \
   "$WORKTREE_ROOT/internal/lsp/bundle/server.js"
```

After this, `go build ./cmd/rwx` will succeed.

## Notes

- Only `server.js` is required for the embed directive; the rest of the bundle
  directory (node_modules, out, support) is not needed.
- Do **not** commit the copied file -- it is gitignored for a reason.
