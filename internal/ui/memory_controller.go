package ui

import (
	"sync"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/memory"
)

var projectMemoryStoreCache struct {
	sync.Mutex
	path string
	store *memory.Store
}

func (m *Model) projectMemoryEnabled() bool {
	if m.cfg == nil {
		return true
	}
	return config.ProjectMemoryEnabled(m.cfg.Paths)
}

func (m *Model) projectMemoryStore() *memory.Store {
	if m.cfg == nil || !m.projectMemoryEnabled() {
		return nil
	}
	dbPath := m.cfg.Paths.MemoryDB()
	projectMemoryStoreCache.Lock()
	defer projectMemoryStoreCache.Unlock()
	if projectMemoryStoreCache.store != nil && projectMemoryStoreCache.path == dbPath {
		return projectMemoryStoreCache.store
	}
	if projectMemoryStoreCache.store != nil {
		_ = projectMemoryStoreCache.store.Close()
		projectMemoryStoreCache.store = nil
	}
	store, err := memory.Open(m.cfg.Paths, memory.DefaultLimits())
	if err != nil {
		return nil
	}
	_ = store.EnsureContextFile()
	projectMemoryStoreCache.path = dbPath
	projectMemoryStoreCache.store = store
	return store
}

func (m *Model) setProjectMemoryEnabled(v bool) error {
	if m.cfg == nil {
		return nil
	}
	if err := config.SetProjectMemory(m.cfg.Paths, v); err != nil {
		return err
	}
	if !v {
		projectMemoryStoreCache.Lock()
		if projectMemoryStoreCache.store != nil {
			_ = projectMemoryStoreCache.store.Close()
			projectMemoryStoreCache.store = nil
			projectMemoryStoreCache.path = ""
		}
		projectMemoryStoreCache.Unlock()
	}
	return nil
}

func (m *Model) configuredProjectMemory() bool {
	return m.projectMemoryEnabled()
}