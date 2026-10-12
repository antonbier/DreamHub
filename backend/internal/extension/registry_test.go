package extension

import "testing"

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if len(r.List()) != 0 {
		t.Fatal("expected an empty registry")
	}
	r.Register(Entry{Registration: Registration{ID: "b", Name: "B"}})
	r.Register(Entry{Registration: Registration{ID: "a", Name: "A"}})
	list := r.List()
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("list = %+v, want sorted a, b", list)
	}

	// Re-registering the same ID replaces, doesn't duplicate.
	r.Register(Entry{Registration: Registration{ID: "a", Name: "A2"}})
	if e, ok := r.Get("a"); !ok || e.Name != "A2" {
		t.Fatalf("Get(a) = %+v, %v", e, ok)
	}
	if len(r.List()) != 2 {
		t.Fatalf("expected re-registration to replace, got %d entries", len(r.List()))
	}

	r.Unregister("a")
	if _, ok := r.Get("a"); ok {
		t.Fatal("expected a to be gone after Unregister")
	}
	if len(r.List()) != 1 {
		t.Fatalf("expected 1 entry left, got %d", len(r.List()))
	}
}
