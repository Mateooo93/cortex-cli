package memory

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

// Store is the project-scoped SQLite memory backend.
type Store struct {
	db      *sql.DB
	project string
	limits  Limits
	paths   config.CortexPaths
}

// Open creates or opens the memory store for paths.Primary().
func Open(paths config.CortexPaths, limits Limits) (*Store, error) {
	if limits.MaxEntries == 0 {
		limits = DefaultLimits()
	}
	project := paths.Primary()
	if project == "" {
		return nil, errors.New("memory: no project .cortex directory")
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		return nil, fmt.Errorf("memory: mkdir: %w", err)
	}
	dbPath := paths.MemoryDB()
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}
	s := &Store{
		db:      db,
		project: project,
		limits:  limits,
		paths:   paths,
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.writeMetadata(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ProjectPath returns the absolute project .cortex directory.
func (s *Store) ProjectPath() string {
	if s == nil {
		return ""
	}
	return s.project
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			type TEXT NOT NULL,
			importance REAL NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			source TEXT NOT NULL,
			project TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("memory: migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) writeMetadata() error {
	count, err := s.Count()
	if err != nil {
		return err
	}
	meta := Metadata{
		Version:     1,
		Project:     s.project,
		MemoryCount: count,
		Retrieval:   "sqlite_like",
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.paths.MemoryMetadata(), data, 0o644)
}

// Count returns the number of stored memories.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n)
	return n, err
}

// Create inserts a new memory after validation and limit checks.
func (s *Store) Create(content string, typ Type, importance float64, source string) (Entry, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Entry{}, errors.New("memory: content is required")
	}
	if !IsValidType(typ) {
		return Entry{}, fmt.Errorf("memory: invalid type %q", typ)
	}
	if importance <= 0 {
		importance = 0.7
	}
	if importance > 1 {
		importance = 1
	}
	if importance < s.limits.MinImportance {
		return Entry{}, fmt.Errorf("memory: importance %.2f below minimum %.2f", importance, s.limits.MinImportance)
	}
	if len(content) > s.limits.MaxContentLen {
		return Entry{}, fmt.Errorf("memory: content exceeds %d characters", s.limits.MaxContentLen)
	}
	if looksTransient(content) {
		return Entry{}, errors.New("memory: content looks temporary; only store durable project facts")
	}
	count, err := s.Count()
	if err != nil {
		return Entry{}, err
	}
	if count >= s.limits.MaxEntries {
		return Entry{}, fmt.Errorf("memory: store full (%d entries); delete old memories first", s.limits.MaxEntries)
	}
	if dup, err := s.findSimilar(content); err != nil {
		return Entry{}, err
	} else if dup != "" {
		return Entry{}, fmt.Errorf("memory: similar entry already exists (%s)", dup)
	}
	if source == "" {
		source = "agent"
	}
	now := time.Now().UTC()
	e := Entry{
		ID:         uuid.NewString(),
		Content:    content,
		Type:       typ,
		Importance: importance,
		CreatedAt:  now,
		UpdatedAt:  now,
		Source:     source,
		Project:    s.project,
	}
	_, err = s.db.Exec(
		`INSERT INTO memories (id, content, type, importance, created_at, updated_at, source, project)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Content, string(e.Type), e.Importance,
		formatTime(e.CreatedAt), formatTime(e.UpdatedAt), e.Source, e.Project,
	)
	if err != nil {
		return Entry{}, err
	}
	_ = s.writeMetadata()
	_ = s.maybeRegenerateContext()
	return e, nil
}

// Update replaces content/type/importance for an existing memory.
func (s *Store) Update(id, content string, typ Type, importance float64) (Entry, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Entry{}, errors.New("memory: content is required")
	}
	if !IsValidType(typ) {
		return Entry{}, fmt.Errorf("memory: invalid type %q", typ)
	}
	if len(content) > s.limits.MaxContentLen {
		return Entry{}, fmt.Errorf("memory: content exceeds %d characters", s.limits.MaxContentLen)
	}
	if looksTransient(content) {
		return Entry{}, errors.New("memory: content looks temporary")
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE memories SET content=?, type=?, importance=?, updated_at=? WHERE id=?`,
		content, string(typ), clampImportance(importance), formatTime(now), id,
	)
	if err != nil {
		return Entry{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Entry{}, fmt.Errorf("memory: not found: %s", id)
	}
	e, err := s.Get(id)
	if err != nil {
		return Entry{}, err
	}
	_ = s.writeMetadata()
	_ = s.maybeRegenerateContext()
	return e, nil
}

// Delete removes a memory by id.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM memories WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: not found: %s", id)
	}
	_ = s.writeMetadata()
	_ = s.maybeRegenerateContext()
	return nil
}

// Get returns one memory by id.
func (s *Store) Get(id string) (Entry, error) {
	row := s.db.QueryRow(
		`SELECT id, content, type, importance, created_at, updated_at, source, project FROM memories WHERE id=?`,
		id,
	)
	return scanEntry(row)
}

// List returns all memories sorted by importance then recency.
func (s *Store) List() ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, content, type, importance, created_at, updated_at, source, project
		 FROM memories ORDER BY importance DESC, updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// Search finds memories matching query (case-insensitive substring).
func (s *Store) Search(query string) ([]Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return s.List()
	}
	q := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(
		`SELECT id, content, type, importance, created_at, updated_at, source, project
		 FROM memories
		 WHERE lower(content) LIKE ? OR lower(type) LIKE ?
		 ORDER BY importance DESC, updated_at DESC`,
		q, q,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *Store) findSimilar(content string) (string, error) {
	needle := strings.ToLower(strings.TrimSpace(content))
	rows, err := s.db.Query(`SELECT id, lower(content) FROM memories`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, existing string
		if err := rows.Scan(&id, &existing); err != nil {
			return "", err
		}
		if existing == needle {
			return id, nil
		}
	}
	return "", rows.Err()
}

func scanEntry(row *sql.Row) (Entry, error) {
	var e Entry
	var typ, created, updated string
	if err := row.Scan(&e.ID, &e.Content, &typ, &e.Importance, &created, &updated, &e.Source, &e.Project); err != nil {
		return Entry{}, err
	}
	e.Type = Type(typ)
	e.CreatedAt = parseTime(created)
	e.UpdatedAt = parseTime(updated)
	return e, nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		var typ, created, updated string
		if err := rows.Scan(&e.ID, &e.Content, &typ, &e.Importance, &created, &updated, &e.Source, &e.Project); err != nil {
			return nil, err
		}
		e.Type = Type(typ)
		e.CreatedAt = parseTime(created)
		e.UpdatedAt = parseTime(updated)
		out = append(out, e)
	}
	return out, rows.Err()
}

// IsValidType reports whether typ is an accepted memory category.
func IsValidType(typ Type) bool {
	for _, t := range ValidTypes {
		if t == typ {
			return true
		}
	}
	return false
}

func clampImportance(v float64) float64 {
	if v <= 0 {
		return 0.7
	}
	if v > 1 {
		return 1
	}
	return v
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

var transientMarkers = []string{
	"today", "tomorrow", "yesterday", "debugging", "current branch",
	"need to fix", "will fix", "todo:", "temporary", "right now",
	"this session", "for now", "later today",
}

func looksTransient(content string) bool {
	lower := strings.ToLower(content)
	for _, m := range transientMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// EnsureContextFile creates an empty starter context.md when missing.
func (s *Store) EnsureContextFile() error {
	path := s.paths.ContextMD()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	starter := "# Project Context\n\n_Add durable project facts via the memory_write tool or /memory._\n"
	return os.WriteFile(path, []byte(starter), 0o644)
}

func (s *Store) maybeRegenerateContext() error {
	count, err := s.Count()
	if err != nil || count == 0 {
		return err
	}
	// Regenerate when count hits multiples of 5 to keep context.md fresh
	// without rewriting on every single memory write.
	if count%5 != 0 {
		return nil
	}
	return RegenerateContext(s)
}

// ReadContextMD returns trimmed context.md contents, capped to ContextMaxBytes.
func (s *Store) ReadContextMD() string {
	data, err := os.ReadFile(s.paths.ContextMD())
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if len(text) > s.limits.ContextMaxBytes {
		text = text[:s.limits.ContextMaxBytes]
		if i := strings.LastIndex(text, "\n"); i > 0 {
			text = text[:i]
		}
		text += "\n…"
	}
	return text
}

// Paths exposes the resolver for UI layers.
func (s *Store) Paths() config.CortexPaths { return s.paths }

// DBPath returns the sqlite file path (for tests).
func (s *Store) DBPath() string { return s.paths.MemoryDB() }

// ContextPath returns context.md path.
func (s *Store) ContextPath() string { return s.paths.ContextMD() }

// Limits returns the active limit set.
func (s *Store) Limits() Limits { return s.limits }

// OpenAt is a test helper that opens a store at an explicit db path.
func OpenAt(dbPath, project string, limits Limits) (*Store, error) {
	if limits.MaxEntries == 0 {
		limits = DefaultLimits()
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	s := &Store{
		db:      db,
		project: project,
		limits:  limits,
		paths:   config.NewCortexPaths(project, "", filepath.Dir(project)),
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}