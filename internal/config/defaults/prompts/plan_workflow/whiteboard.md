You have just produced an implementation plan. Generate a visual whiteboard representation of it so the user can walk through the changes before execution.

## The plan

$(plan)

---

## Your task

Produce a JSON array of scenes that visualises this plan. Output **only** the raw JSON array — no preamble, no explanation, no markdown fences.

## Scene grouping

Do **not** create one scene per step. Group related steps into scenes that tell a coherent story:

- Steps that touch the same layer (UI, API, database, config) belong together on one scene
- Steps with a tight dependency chain work well as a single flow diagram
- A typical plan needs **2–3 scenes**, not one per step

Good scene names: `"Data Model"`, `"API Layer"`, `"Frontend Changes"`, `"Auth Flow"`, `"Before / After"`.

## What to put in each scene

Use **nodes and edges** to show architecture, data flow, or component relationships for that group of steps. Always leave the `code` array empty — do not include any code artifacts.

---

## Scenes JSON Schema

The top level is an **array of scene objects**:

```json
[
  {
    "name": "Scene 1",
    "context": "...",
    "nodes": [],
    "edges": [],
    "code": []
  }
]
```

### Scene

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Display name shown in the scene list. E.g. `"Architecture Overview"`, `"Auth Flow"`. |
| `context` | string | yes | 1–3 sentences for the ElevenLabs agent explaining what this scene depicts and how it relates to the plan. |
| `nodes` | array | yes | Array of node objects for this scene's canvas. May be empty. |
| `edges` | array | yes | Array of edge objects for this scene's canvas. May be empty. |
| `code` | array | yes | Always `[]`. |

### Node

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Stable unique identifier. Use format `"node_N"` (e.g. `"node_1"`). Never change an ID across calls — edges reference nodes by ID. |
| `shape` | string | yes | One of `"rectangle"`, `"diamond"`, `"database"`, `"textbox"`. See shapes below. |
| `x` | number | yes | Horizontal position of the node's top-left corner in canvas units. Nodes between 0–800 are visible without panning. |
| `y` | number | yes | Vertical position of the node's top-left corner. Nodes between 0–600 are visible without panning. |
| `label` | string | yes | Text displayed inside the node. Keep short (1–3 words) for shapes; textbox supports longer text and `\n`. |
| `color` | string | yes | Fill/background color as a hex code. See color palette below. |
| `border_color` | string | yes* | Border color. *Ignored for `textbox`. Pick one shade darker than `color`. |
| `text_alignment` | string | yes | One of `"left"`, `"center"`, `"right"`. Only visually effective on `textbox`; use `"center"` for all other shapes. |

**Shapes:**
- **`rectangle`** — Filled rectangle with rounded corners and a 2px border. ~150×80px. Ideal for services, components.
- **`diamond`** — Rotated square rendered as SVG. ~128×128px. Ideal for decision points, load balancers.
- **`database`** — 3D cylinder rendered as SVG. ~128×140px. Ideal for databases, queues, storage.
- **`textbox`** — Plain text label, no border, transparent background by default. Ideal for annotations, titles, section labels.

**Color palette** (use only these values):

| Hex | Name | | Hex | Name |
|-----|---------|-|-----|------|
| `#ef4444` | red | | `#0ea5e9` | sky |
| `#f97316` | orange | | `#3b82f6` | blue *(rectangle default)* |
| `#f59e0b` | amber | | `#6366f1` | indigo |
| `#eab308` | yellow | | `#8b5cf6` | violet *(diamond default)* |
| `#84cc16` | lime | | `#a855f7` | purple |
| `#22c55e` | green | | `#d946ef` | fuchsia |
| `#10b981` | emerald *(database default)* | | `#ec4899` | pink |
| `#14b8a6` | teal | | `#64748b` | slate |
| `#06b6d4` | cyan | | `#6b7280` | gray |
| `#000000` | black | | `#ffffff` | white |
| `transparent` | *(textbox only)* | | | |

### Edge

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Stable unique identifier. Use format `"edge_N"` (e.g. `"edge_1"`). |
| `from` | string | yes | ID of the source node. |
| `from_handle` | string | yes | Side the arrow exits from: `"top"`, `"bottom"`, `"left"`, or `"right"`. |
| `to` | string | yes | ID of the target node. |
| `to_handle` | string | yes | Side the arrow enters: `"top"`, `"bottom"`, `"left"`, or `"right"`. |

Edges are rendered as animated dashed black arrows. Color and style cannot be changed.

---

## Layout guidance

- Space nodes at least **200px apart** (x or y) to avoid overlap.
- For a **left-to-right** diagram, increment `x` by ~250–350px per layer; keep `y` consistent within a layer.
- For a **top-to-bottom** diagram, increment `y` by ~200–250px per layer.
- A typical system design fits comfortably in `x: 0–1200`, `y: 0–800`.
- Use `textbox` nodes as section headers or floating labels near groups of nodes.

---

## Example

```json
[
  {
    "name": "Architecture",
    "context": "...",
    "nodes": [
      {
        "id": "node_1",
        "shape": "rectangle",
        "x": 100, "y": 200,
        "label": "API Gateway",
        "color": "#3b82f6",
        "border_color": "#1d4ed8",
        "text_alignment": "center"
      },
      {
        "id": "node_2",
        "shape": "database",
        "x": 450, "y": 200,
        "label": "PostgreSQL",
        "color": "#10b981",
        "border_color": "#059669",
        "text_alignment": "center"
      }
    ],
    "edges": [
      {
        "id": "edge_1",
        "from": "node_1", "from_handle": "right",
        "to": "node_2", "to_handle": "left"
      }
    ],
    "code": []
  },
  {
    "name": "Auth Flow",
    "context": "...",
    "nodes": [
      {
        "id": "node_3",
        "shape": "rectangle",
        "x": 100, "y": 200,
        "label": "Client",
        "color": "#6366f1",
        "border_color": "#4338ca",
        "text_alignment": "center"
      },
      {
        "id": "node_4",
        "shape": "diamond",
        "x": 400, "y": 180,
        "label": "Auth?",
        "color": "#8b5cf6",
        "border_color": "#6d28d9",
        "text_alignment": "center"
      }
    ],
    "edges": [
      {
        "id": "edge_2",
        "from": "node_3", "from_handle": "right",
        "to": "node_4", "to_handle": "left"
      }
    ],
    "code": []
  }
]
```
