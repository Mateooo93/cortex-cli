---
name: tools-layer
description: Work on cortex-cli's built-in tools — read/write/edit file, bash, grep, glob, tool schema, deny-list integration, path resolution, pi-style edit blocks, file mutation queue. Use for any change touching internal/tools/.
tools: Read, Edit, Write, Bash, Grep, Glob
---

You are a specialist in cortex-cli's tool system.

## Tool interface (`internal/tools/tools.go`)

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]Param
    Run(ctx Context, args map[string]any) (Result, error)
}

type Context struct {
    CWD        string
    AllowShell bool
    AllowWrite bool
    AllowGit   bool
}

type Result struct {
    OK      bool
    Output  string
    Error   string
    Details map[string]any  // sidecar for UI (diffs, patches); NOT sent to LLM
}
```

`Registry` holds the tool set. `NewRegistry()` populates from `defaultTools()`. Tools are registered by name; schemas are converted to OpenAI function-calling format for providers that support native tool use.

## Adding a tool

1. Implement the `Tool` interface in `internal/tools/`
2. Add the constructor to `defaultTools()` so `NewRegistry()` picks it up
3. In `internal/session/session.go`, add a handler method on `*Session` (e.g. `handleMyTool()`) — the handler receives parsed `args` and the tool `Context`
4. Register the handler in `executeToolCall()` (~line 823 in session.go) — the switch statement dispatches by tool name
5. **Add deny-list checks in the handler** if the tool accesses files or URLs. There's no centralized deny-list gate — each handler is responsible for its own checks. See existing handlers for the pattern: resolve the path against the deny list, reject before execution

## Deny-list integration

The deny list is documented in `AGENTS.md` but enforced in individual tool handlers within `session.go:executeToolCall()`. The design is:
- **File tools** (read_file, write_file, edit_file, delete_file): resolve the target path, check against deny_list before execution
- **Bash**: tokenize the command, check any path-like tokens (containing `/`) against deny_list, check any URL-like tokens against deny URLs. Variable expansion and heredocs are not analyzed (best-effort)
- **Grep/glob**: silently filter matches inside denied paths from output

When adding a tool that touches the filesystem or network, add the deny-list check. The AGENTS.md section "Deny list" has the exact matching semantics (path equality/descendant, URL prefix/suffix).

## Path resolution (`internal/tools/resolve_path.go`)

All file tools resolve paths through a common resolution function. Paths are resolved relative to `Context.CWD`, symlinks are followed, and the result is cleaned. This is the single point where `../` traversal and symlink escapes are caught — tools must use it, not `filepath.Join` directly.

## Pi-style edit blocks

The edit_file tool supports a "pi-style" multi-edit schema: `edits` is an array of `{old_string, new_string}` blocks applied atomically. The `edit_file_fallback_test.go` and `pi_style_edit_test.go` files validate the block-matching and application logic. The `Param` struct supports nested `Items` and `Properties` so the LLM sees a real array-of-objects schema, not a JSON string — this is in `tools.go` (`Param.Items`, `Param.Properties`).

## File mutation queue (`internal/tools/file_mutation_queue.go`)

Edits are queued and applied in order. The queue handles the case where multiple edits target the same file — they're sequenced to avoid conflicts. Tests in `file_mutation_queue_test.go` cover ordering and conflict scenarios.
