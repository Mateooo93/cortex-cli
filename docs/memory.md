# Project-scoped memory

Cortex CLI stores durable project knowledge under the active repository's `.cortex/` directory. Memories from one project never appear in another.

## Layout

```text
my-project/.cortex/
├── memory.db      # SQLite store (CRUD + search)
├── context.md     # Compact human-readable summary (<1 KB target)
└── metadata.json  # Version, counts, retrieval backend id
```

## Categories

| Type | Example |
|------|---------|
| `preference` | Use uv instead of pip |
| `convention` | TypeScript strict mode everywhere |
| `architecture` | Repository pattern for data access |
| `workflow` | Run `make test` before committing |
| `project_fact` | Frontend uses React Query |

Temporary session facts (bugs in progress, branch names, todos) must not be saved.

## Agent tool: `memory_write`

The model can persist memories via the `memory_write` tool. Limits:

- Max **100** memories per project
- Max **500** characters per memory
- Min importance **0.55** (use ≥0.75 for high-value facts)
- Duplicate / transient content is rejected

## Prompt injection

Before each turn, Cortex loads:

1. `context.md` (capped)
2. Top **8** relevant memories (≤ **2 KB** total)

Relevance uses the current user message as a hint. The full database is never injected.

## UI

- `/memory` — searchable modal browser (delete with `d`, expand with Enter)
- **Settings → Other Settings → Project memory** — toggle on/off (default: on)

## Future retrieval backends

`internal/memory.Retriever` is the extension point for embeddings (pgvector, Chroma, LanceDB, Qdrant) without changing the CLI surface.