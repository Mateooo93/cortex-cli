package config

import "testing"

func TestDefaultAgents_IncludesGeneral(t *testing.T) {
	catalog, err := DefaultAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) == 0 {
		t.Fatal("expected embedded default agents")
	}
	if _, ok := catalog["general"]; !ok {
		t.Fatal("expected general agent in defaults")
	}
	if _, ok := catalog["implementer"]; !ok {
		t.Fatal("expected implementer agent in defaults")
	}
}