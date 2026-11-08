package pad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	toolName    = "ch57x-keyboard-tool"
	toolTimeout = 10 * time.Second
)

// runFunc executes the tool and returns its stderr. It is a seam for tests.
type runFunc func(ctx context.Context, path string, args []string, stdin []byte) (stderr string, err error)

// ToolWriter implements Writer by running ch57x-keyboard-tool.
type ToolWriter struct {
	// Path to the tool; empty means look up "ch57x-keyboard-tool" on PATH.
	Path string
	// ProductID, when non-zero and not 0x8890, is passed as --product-id.
	ProductID uint16

	run      runFunc                      // nil means execRun
	lookPath func(string) (string, error) // nil means exec.LookPath
}

// NewToolWriter returns a ToolWriter using PATH lookup.
func NewToolWriter() *ToolWriter { return &ToolWriter{} }

func (w *ToolWriter) resolve() (string, error) {
	if w.Path != "" {
		p, err := exec.LookPath(w.Path)
		if err != nil {
			return "", fmt.Errorf("%w: %s", ErrToolMissing, w.Path)
		}
		return p, nil
	}
	look := w.lookPath
	if look == nil {
		look = exec.LookPath
	}
	p, err := look(toolName)
	if err != nil {
		return "", ErrToolMissing
	}
	return p, nil
}

// Available reports ErrToolMissing if the tool cannot be found.
func (w *ToolWriter) Available() error {
	_, err := w.resolve()
	return err
}

// Upload writes a mapping YAML document to the pad.
func (w *ToolWriter) Upload(mapping []byte) error {
	return w.exec(mapping, "upload")
}

// SetLED validates s against CapsFor(w.ProductID) (0 means 8890) and applies it.
// Invalid settings are rejected before the tool runs.
func (w *ToolWriter) SetLED(s LEDSetting) error {
	caps := CapsFor(w.ProductID)
	if err := ValidateLED(caps, s); err != nil {
		return err
	}
	return w.exec(nil, append([]string{"led"}, ledArgs(caps, s)...)...)
}

func (w *ToolWriter) exec(stdin []byte, args ...string) error {
	path, err := w.resolve()
	if err != nil {
		return err
	}
	var full []string
	if w.ProductID != 0 && w.ProductID != 0x8890 {
		// clap parses the option as a decimal u16.
		full = append(full, "--product-id", strconv.Itoa(int(w.ProductID)))
	}
	full = append(full, args...)

	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	run := w.run
	if run == nil {
		run = execRun
	}
	stderr, err := run(ctx, path, full, stdin)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%s %s: timed out after %s", toolName, args[0], toolTimeout)
	}
	return mapToolError(args[0], stderr, err)
}

func execRun(ctx context.Context, path string, args []string, stdin []byte) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// mapToolError classifies tool failures. Observed v1.7.0 output with no pad:
//
//	Error: find USB device
//
//	Caused by:
//	    CH57x keyboard device not found. Use --vendor-id and --product-id to override settings.
//
// Permission problems surface from rusb/libusb as "Access denied (insufficient permissions)".
func mapToolError(op, stderr string, err error) error {
	detail := strings.TrimSpace(stderr)
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "access denied"),
		strings.Contains(lower, "insufficient permissions"),
		strings.Contains(lower, "permission denied"):
		return fmt.Errorf("%w: %s", ErrPermission, detail)
	case strings.Contains(lower, "device not found"),
		strings.Contains(lower, "find usb device"),
		strings.Contains(lower, "no such device"):
		return fmt.Errorf("%w: %s", ErrNotConnected, detail)
	}
	var exitErr *exec.ExitError
	if detail == "" && !errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s: %w", toolName, op, err)
	}
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("%s %s failed: %s", toolName, op, detail)
}
