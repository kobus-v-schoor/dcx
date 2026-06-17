package log

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestHandlerPlainText(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)

	logger.Info("hello world", "key", "val")

	out := buf.String()
	if !strings.Contains(out, "INFO ") {
		t.Errorf("expected INFO in output, got %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", out)
	}
	if !strings.Contains(out, "key=val") {
		t.Errorf("expected key=val in output, got %q", out)
	}
	// Plain text on a non-terminal writer should include a timestamp.
	if !strings.Contains(out, ":") {
		t.Errorf("expected timestamp in plain-text output, got %q", out)
	}
}

func TestHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h.WithAttrs([]slog.Attr{slog.String("app", "dcx")}))

	logger.Warn("something happened")

	out := buf.String()
	if !strings.Contains(out, "app=dcx") {
		t.Errorf("expected app=dcx in output, got %q", out)
	}
	if !strings.Contains(out, "something happened") {
		t.Errorf("expected 'something happened' in output, got %q", out)
	}
}

func TestHandlerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(h)

	logger.Info("should not appear")
	logger.Warn("should appear")

	out := buf.String()
	if strings.Contains(out, "should not appear") {
		t.Errorf("expected INFO message to be filtered out")
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("expected WARN message, got %q", out)
	}
}

func TestHandlerQuotesWhitespace(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)

	logger.Info("msg", "path", "/foo bar/baz")

	out := buf.String()
	if !strings.Contains(out, `path="/foo bar/baz"`) {
		t.Errorf("expected quoted value with spaces, got %q", out)
	}
}

func TestHandlerColorOutput(t *testing.T) {
	var buf bytes.Buffer
	h := &Handler{out: &buf, level: slog.LevelInfo, useColor: true}
	logger := slog.New(h)

	logger.Info("hello", "k", "v")
	logger.Warn("careful")
	logger.Error("oops")

	out := buf.String()
	if !strings.Contains(out, cyan) {
		t.Errorf("expected cyan color for INFO, got %q", out)
	}
	if !strings.Contains(out, yellow) {
		t.Errorf("expected yellow color for WARN, got %q", out)
	}
	if !strings.Contains(out, red) {
		t.Errorf("expected red color for ERROR, got %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "\x1b[") {
			t.Errorf("expected line to start with ANSI escape, got %q", line)
		}
	}
}
