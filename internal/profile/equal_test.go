package profile

import "testing"

func TestSaveIdenticalToBuiltinRemovesOverride(t *testing.T) {
	st := NewFileStore(t.TempDir())
	codex, err := st.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	edited := codex
	edited.Keys[0] = "enter"
	if err := st.Save(edited); err != nil {
		t.Fatal(err)
	}
	if !st.IsOverridden("codex") {
		t.Fatal("real edit should create an override")
	}
	if err := st.Save(codex); err != nil {
		t.Fatal(err)
	}
	if st.IsOverridden("codex") {
		t.Fatal("saving the built-in values should remove the override")
	}
	if err := st.Save(codex); err != nil {
		t.Fatal("saving built-in values without an override must not fail:", err)
	}
}

func TestEqualTreatsEmptyGesturesAsNil(t *testing.T) {
	a := Profile{Name: "x", Keys: [3]string{"1", "2", "3"}, Knob: Knob{"up", "enter", "down"}}
	b := a
	b.Gestures = map[string]map[string]string{"key1": {"hold": ""}}
	if !Equal(a, b) {
		t.Fatal("empty gesture actions should not make profiles differ")
	}
	b.Gestures = map[string]map[string]string{"key1": {"hold": "2"}}
	if Equal(a, b) {
		t.Fatal("a real gesture action must make profiles differ")
	}
}
