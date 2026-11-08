package settings

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/profile"
)

func TestExportProfile(t *testing.T) {
	data, err := ExportProfile(zellij)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "kind: agentpad-profile\nschema: 1\nname: zellij-claude\n") || !strings.Contains(text, "ctrl-c") {
		t.Fatalf("export:\n%s", text)
	}
	p, err := ParseProfile(data)
	if err != nil || !reflect.DeepEqual(p, zellij) {
		t.Fatalf("parse = %+v, %v", p, err)
	}
}

func TestImportProfile(t *testing.T) {
	codex := profile.Builtins()[1]
	codex.Keys[0] = "enter"
	shared, _ := ExportProfile(codex)
	other := codex
	other.Keys[1] = "n"
	sharedOther, _ := ExportProfile(other)

	tests := []struct {
		name     string
		existing []string // overrides saved first (codex variants)
		data     []byte
		want     string
		wantErr  string
	}{
		{"builtin not overridden", nil, shared, "codex", ""},
		{"conflict", []string{"codex"}, sharedOther, "codex-2", ""},
		{"second conflict", []string{"codex", "codex-2"}, sharedOther, "codex-3", ""},
		{"identical is a no-op", []string{"codex"}, shared, "codex", ""},
		{"plain profile yaml", nil, []byte("name: mine\nkeys: [a, b, c]\nknob: {ccw: up, press: enter, cw: down}\n"), "mine", ""},
		{"invalid key", nil, []byte("kind: agentpad-profile\nschema: 1\nname: bad\nkeys: [a, foo, c]\nknob: {ccw: up, press: enter, cw: down}\n"), "", `profile bad: key 2: unknown key "foo"`},
		{"bundle", nil, []byte("kind: agentpad-bundle\nschema: 1\n"), "", "bundle"},
		{"newer", nil, []byte("kind: agentpad-profile\nschema: 3\nname: x\n"), "", "newer"},
		{"garbage", nil, []byte("::: ["), "", "line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := profile.NewFileStore(filepath.Join(t.TempDir(), "profiles"))
			for i, n := range tt.existing {
				p := codex
				p.Name = n
				if i > 0 {
					p.Keys[2] = "q"
				}
				if err := store.Save(p); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ImportProfile(tt.data, store)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if !store.IsOverridden("bad") && len(tt.existing) == 0 {
					return
				}
				t.Fatal("invalid profile saved")
			}
			if err != nil || got.Name != tt.want || !store.IsOverridden(tt.want) {
				t.Fatalf("import = %+v, %v", got, err)
			}
			if saved, _ := store.Get(tt.want); !reflect.DeepEqual(saved, got) {
				t.Fatalf("saved = %+v", saved)
			}
		})
	}
}
