package ui

// Settings export / import / share helpers without Fyne: the import preview
// model, readable error text, file names and the profiles dir signature.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/thereisnotime/agentpad/internal/profile"
	"github.com/thereisnotime/agentpad/internal/settings"
)

// rowKind colours a preview row.
type rowKind int

const (
	rowAdd rowKind = iota
	rowChange
	rowRemove
	rowWarning
)

// previewRow is one line of the import preview.
type previewRow struct {
	Kind   rowKind
	Text   string
	Detail string // optional second line (old -> new for settings)
}

// previewGroup is one area of the import preview.
type previewGroup struct {
	Title string
	Rows  []previewRow
}

// planPreview turns an import plan into grouped rows: Settings, Profiles,
// Rules, Scripts, Hooks (adds, then changes, then removals, each in plan
// order), then Warnings. Empty groups are left out. Dangerous changes
// (scripts and hooks that would run commands) are not here: see
// consentRows.
func planPreview(p *settings.Plan) []previewGroup {
	if p == nil {
		return nil
	}
	areas := []struct{ area, title string }{
		{settings.AreaSettings, "Settings"},
		{settings.AreaProfile, "Profiles"},
		{settings.AreaRules, "Rules"},
		{settings.AreaScripts, "Scripts"},
		{settings.AreaHooks, "Hooks"},
	}
	var out []previewGroup
	for _, a := range areas {
		var rows []previewRow
		for _, k := range []settings.ChangeKind{settings.Add, settings.Modify, settings.Remove} {
			for _, c := range p.Changes {
				if c.Area == a.area && c.Kind == k && !c.Dangerous {
					rows = append(rows, changeRow(c))
				}
			}
		}
		if len(rows) > 0 {
			out = append(out, previewGroup{Title: a.title, Rows: rows})
		}
	}
	if len(p.Warnings) > 0 {
		g := previewGroup{Title: "Warnings"}
		for _, w := range p.Warnings {
			g.Rows = append(g.Rows, previewRow{Kind: rowWarning, Text: w})
		}
		out = append(out, g)
	}
	return out
}

func changeRow(c settings.Change) previewRow {
	switch c.Area {
	case settings.AreaSettings:
		switch c.Kind {
		case settings.Add:
			return previewRow{Kind: rowAdd, Text: c.Path, Detail: "set to " + display(c.New)}
		case settings.Remove:
			return previewRow{Kind: rowRemove, Text: c.Path, Detail: "was " + display(c.Old)}
		}
		return previewRow{Kind: rowChange, Text: c.Path, Detail: display(c.Old) + " → " + display(c.New)}
	case settings.AreaProfile:
		switch c.Kind {
		case settings.Add:
			return previewRow{Kind: rowAdd, Text: c.Path, Detail: "new profile"}
		case settings.Remove:
			if profile.IsBuiltin(c.Path) {
				return previewRow{Kind: rowRemove, Text: c.Path, Detail: "your changes are dropped, the built-in comes back"}
			}
			return previewRow{Kind: rowRemove, Text: c.Path, Detail: "profile is deleted"}
		}
		return previewRow{Kind: rowChange, Text: c.Path, Detail: "keys or description change"}
	case settings.AreaScripts:
		switch c.Kind {
		case settings.Add:
			return previewRow{Kind: rowAdd, Text: "script " + c.Path, Detail: c.New}
		case settings.Remove:
			return previewRow{Kind: rowRemove, Text: "script " + c.Path, Detail: "removed (was " + c.Old + ")"}
		}
		return previewRow{Kind: rowChange, Text: "script " + c.Path, Detail: c.Old + " → " + c.New}
	case settings.AreaHooks:
		if c.Kind == settings.Remove {
			return previewRow{Kind: rowRemove, Text: "hook " + c.Path, Detail: "removed"}
		}
		return previewRow{Kind: rowAdd, Text: "hook " + c.Path, Detail: "runs the script on this event"}
	}
	kinds := map[settings.ChangeKind]rowKind{settings.Add: rowAdd, settings.Modify: rowChange, settings.Remove: rowRemove}
	verb := map[settings.ChangeKind]string{settings.Add: "added", settings.Modify: "replaced", settings.Remove: "removed"}
	return previewRow{Kind: kinds[c.Kind], Text: c.Path, Detail: verb[c.Kind]}
}

func display(v string) string {
	if v == "" {
		return `""`
	}
	return v
}

// consentRow is one dangerous change, ticked by the user to import it.
type consentRow struct {
	Change settings.Change
	Row    previewRow
}

// consentRows lists the plan's dangerous changes for the "Runs commands"
// section; all start unticked.
func consentRows(p *settings.Plan) []consentRow {
	if p == nil {
		return nil
	}
	var out []consentRow
	for _, c := range p.Dangerous() {
		out = append(out, consentRow{Change: c, Row: changeRow(c)})
	}
	return out
}

// consentPlan is the plan to apply given the ticked dangerous changes
// (keyed by Change.Area + "/" + Change.Path).
func consentPlan(p *settings.Plan, ticked map[string]bool) *settings.Plan {
	return p.Consent(func(c settings.Change) bool { return ticked[consentKey(c)] })
}

func consentKey(c settings.Change) string { return c.Area + "/" + c.Path }

// planSummary is the one-line count above the preview.
func planSummary(p *settings.Plan) string {
	if p == nil || p.Empty() {
		return "Nothing would change."
	}
	return fmt.Sprintf("%d added · %d changed · %d removed",
		p.Count(settings.Add), p.Count(settings.Modify), p.Count(settings.Remove))
}

// errorText renders an error for a dialog: one problem per line for
// validation errors (field or profile and control, with the line).
func errorText(err error) string {
	var ve *settings.ValidationError
	if !errors.As(err, &ve) || len(ve.Errors) == 0 {
		return err.Error()
	}
	lines := make([]string, len(ve.Errors))
	for i, fe := range ve.Errors {
		lines[i] = "• " + fe.Error()
	}
	head := "Problems found"
	if ve.File != "" {
		head += " in " + filepath.Base(ve.File)
	}
	return head + ":\n" + strings.Join(lines, "\n")
}

// configPillText is the short header pill text for a settings error.
func configPillText(err error) string {
	var ve *settings.ValidationError
	if errors.As(err, &ve) && len(ve.Errors) > 0 {
		if l := ve.Errors[0].Line; l > 0 {
			return fmt.Sprintf("settings.yaml: error on line %d", l)
		}
		return "settings.yaml: " + ve.Errors[0].Path
	}
	return "settings.yaml: error"
}

// exportFileName is the suggested bundle name for day t.
func exportFileName(t time.Time) string {
	return "agentpad-" + t.Format("2006-01-02") + settings.BundleExt
}

// profileFileName is the suggested file name for a shared profile.
func profileFileName(name string) string { return name + ".agentpad-profile.yaml" }

// shareImportMessage describes a profile import: renamed, unchanged or new.
func shareImportMessage(asked, saved string, existedSame bool) string {
	switch {
	case asked != saved:
		return fmt.Sprintf("Imported as %s: you already have a different %s.", saved, asked)
	case existedSame:
		return fmt.Sprintf("You already have this exact %s profile, nothing changed.", saved)
	}
	return "Imported " + saved + "."
}

// backupLabel is a backup's line in the Backups list (local time).
func backupLabel(b settings.Backup) string {
	return fmt.Sprintf("%s · %s", b.Time.Local().Format("2006-01-02 15:04:05"), plural(len(b.Files), "file"))
}

// dirSignature summarises the *.yaml files in dir (names, sizes, mtimes);
// "" when dir is empty, missing or unreadable.
func dirSignature(dir string) string {
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var parts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", e.Name(), fi.Size(), fi.ModTime().UnixNano()))
	}
	slices.Sort(parts)
	return strings.Join(parts, "|")
}
