package memory

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestStore_CreateListDelete(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	s, err := OpenAt(dbPath, dir, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	e, err := s.Create("Use uv instead of pip", TypePreference, 0.9, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" {
		t.Fatal("expected id")
	}
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, err = %v", list, err)
	}
	found, err := s.Search("uv")
	if err != nil || len(found) != 1 {
		t.Fatalf("search = %v, err = %v", found, err)
	}
	if err := s.Delete(e.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Count(); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestStore_RejectsTransientContent(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenAt(filepath.Join(dir, "memory.db"), dir, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Create("Debugging auth issue today", TypeProjectFact, 0.9, "agent"); err == nil {
		t.Fatal("expected transient rejection")
	}
}

func TestStore_EnforcesMaxEntries(t *testing.T) {
	dir := t.TempDir()
	limits := DefaultLimits()
	limits.MaxEntries = 2
	s, err := OpenAt(filepath.Join(dir, "memory.db"), dir, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Create("First durable fact", TypeProjectFact, 0.8, "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("Second durable fact", TypeProjectFact, 0.8, "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("Third durable fact", TypeProjectFact, 0.8, "agent"); err == nil {
		t.Fatal("expected max entries error")
	}
}

func TestStore_ProjectIsolation(t *testing.T) {
	aDir := t.TempDir()
	bDir := t.TempDir()
	a, err := OpenAt(filepath.Join(aDir, "memory.db"), aDir, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenAt(filepath.Join(bDir, "memory.db"), bDir, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := a.Create("Project A uses FastAPI", TypeArchitecture, 0.9, "agent"); err != nil {
		t.Fatal(err)
	}
	bList, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(bList) != 0 {
		t.Fatalf("project B should be empty, got %d", len(bList))
	}
}

func TestRetrieveRelevant_RespectsBudget(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenAt(filepath.Join(dir, "memory.db"), dir, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 12; i++ {
		_, err := s.Create(fmt.Sprintf("Durable convention number %d", i), TypeConvention, 0.7+float64(i)*0.01, "agent")
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.RetrieveRelevant("", 5, 512)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 5 {
		t.Fatalf("expected <=5 entries, got %d", len(entries))
	}
}