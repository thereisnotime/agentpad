package ui

import (
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/thereisnotime/agentpad/internal/pad"
)

const (
	repoURL       = "https://github.com/thereisnotime/agentpad"
	docsURL       = repoURL + "#readme"
	releasesURL   = repoURL + "/releases"
	newIssueURL   = repoURL + "/issues/new"
	featureReqURL = newIssueURL + "?template=feature_request.yml"
)

// diagnostics is what a bug report needs to know about this install.
type diagnostics struct {
	Version string
	OS      string // "linux/amd64 · <kernel> · <session>"
	Pad     string // "1189:8890" or "not connected"
}

// osString formats the OS line of a bug report.
func osString(goos, goarch, kernel, session string) string {
	parts := []string{goos + "/" + goarch}
	if kernel = strings.TrimSpace(kernel); kernel != "" {
		parts = append(parts, kernel)
	}
	if session != "" {
		parts = append(parts, session)
	}
	return strings.Join(parts, " · ")
}

// padString formats the pad line of a bug report.
func padString(d pad.Device, present bool) string {
	if !present {
		return "not connected"
	}
	return fmt.Sprintf("%04x:%04x", d.VendorID, d.ProductID)
}

// localOS describes the running system (kernel from /proc, like uname -r).
func localOS() string {
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	return osString(runtime.GOOS, runtime.GOARCH, string(kernel), os.Getenv("XDG_SESSION_TYPE"))
}

// bugReportURL opens the bug template with the diagnostics prefilled.
func bugReportURL(d diagnostics) string {
	q := url.Values{}
	q.Set("template", "bug_report.yml")
	q.Set("version", d.Version)
	q.Set("os", d.OS)
	q.Set("pad", d.Pad)
	q.Set("title", "[bug] ")
	return newIssueURL + "?" + q.Encode()
}

// text is the plain-text form copied by "Copy diagnostics".
func (d diagnostics) text() string {
	return fmt.Sprintf("agentpad %s\nOS: %s\nPad: %s\n", d.Version, d.OS, d.Pad)
}
