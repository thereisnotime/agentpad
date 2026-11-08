package desktopentry

import (
	"os"
	"strings"
	"testing"

	"github.com/thereisnotime/agentpad/internal/config"
)

func TestEnsureInstallsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	changed, err := Ensure(dir, "/opt/agentpad/bin/agentpad")
	if err != nil || !changed {
		t.Fatalf("first Ensure: changed=%v err=%v", changed, err)
	}
	p := PathsIn(dir)
	entry, err := os.ReadFile(p.Desktop)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Exec=/opt/agentpad/bin/agentpad", "Icon=" + config.AppID, "StartupWMClass=" + config.AppID} {
		if !strings.Contains(string(entry), want) {
			t.Errorf("entry missing %q:\n%s", want, entry)
		}
	}
	for _, f := range []string{p.SVG, p.PNG} {
		if st, err := os.Stat(f); err != nil || st.Size() == 0 {
			t.Errorf("icon %s not written: %v", f, err)
		}
	}
	if changed, err := Ensure(dir, "/opt/agentpad/bin/agentpad"); err != nil || changed {
		t.Fatalf("second Ensure should be a no-op: changed=%v err=%v", changed, err)
	}
	if changed, _ := Ensure(dir, "/usr/local/bin/agentpad"); !changed {
		t.Fatal("Ensure should rewrite when Exec changes")
	}
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.Desktop); !os.IsNotExist(err) {
		t.Fatal("desktop file not removed")
	}
}

func TestQuoteExec(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/agentpad":  "/usr/bin/agentpad",
		"/home/a b/agentpad": `"/home/a b/agentpad"`,
		`/x/$weird"name`:     `"/x/\$weird\"name"`,
	}
	for in, want := range cases {
		if got := quoteExec(in); got != want {
			t.Errorf("quoteExec(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmbeddedIconsMatchAssets(t *testing.T) {
	for emb, asset := range map[string][]byte{"../../assets/agentpad.svg": iconSVG, "../../assets/agentpad-256.png": iconPNG} {
		b, err := os.ReadFile(emb)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(asset) {
			t.Errorf("embedded icon differs from %s; copy the asset into internal/desktopentry", emb)
		}
	}
}
