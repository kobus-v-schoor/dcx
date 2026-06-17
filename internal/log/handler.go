// Package log provides a pretty, colourised slog.Handler for interactive CLI
// use. When the output stream is not a terminal it falls back to plain text
// without colour codes.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ANSI colour / style escapes.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
	gray   = "\x1b[90m"
)

// Handler is a slog.Handler optimised for human readability in a terminal.
// It prints a short, left-aligned, bold level tag, the message, and any
// attributes as dimmed key=value pairs. When the destination is not a TTY,
// colour is disabled and a brief timestamp is included.
type Handler struct {
	out      io.Writer
	mu       sync.Mutex
	level    slog.Level
	useColor bool
	attrs    []slog.Attr
	groups   []string
}

// NewHandler returns a Handler writing to w. If w is an *os.File connected
// to a terminal, the output is colourised and timestamps are omitted so
// messages fit cleanly on a single line. Otherwise plain text without
// escape codes and with a short timestamp is used.
func NewHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	level := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		level = opts.Level.Level()
	}

	useColor := false
	if f, ok := w.(*os.File); ok {
		useColor = term.IsTerminal(int(f.Fd()))
	}

	return &Handler{
		out:      w,
		level:    level,
		useColor: useColor,
	}
}

// Enabled reports whether the handler should process records at the given
// level.
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

// Handle formats a slog.Record and writes it to the output stream.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	// Build the flat slice of effective attributes. Prepend any WithAttrs.
	effective := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	effective = append(effective, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		effective = append(effective, a)
		return true
	})

	if h.useColor {
		// Omit timestamp for interactive brevity.
		b.WriteString(levelStyle(r.Level))
		b.WriteString(bold)
		b.WriteString(levelString(r.Level))
		b.WriteString(reset)
		b.WriteByte(' ')
		b.WriteString(r.Message)

		if len(effective) > 0 {
			b.WriteString(dim)
			for _, a := range effective {
				b.WriteByte(' ')
				writeAttr(&b, a)
			}
			b.WriteString(reset)
		}
	} else {
		// Plain text: keep a short timestamp for file/logging compatibility.
		b.WriteString(r.Time.Format(time.TimeOnly))
		b.WriteByte(' ')
		b.WriteString(levelString(r.Level))
		b.WriteByte(' ')
		b.WriteString(r.Message)

		for _, a := range effective {
			b.WriteByte(' ')
			writeAttr(&b, a)
		}
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write([]byte(b.String()))
	return err
}

// WithAttrs returns a new Handler that includes the given attributes in
// every record.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := &Handler{
		out:      h.out,
		level:    h.level,
		useColor: h.useColor,
		attrs:    append(make([]slog.Attr, 0, len(h.attrs)+len(attrs)), h.attrs...),
		groups:   append(make([]string, 0, len(h.groups)), h.groups...),
	}
	h2.attrs = append(h2.attrs, attrs...)
	return h2
}

// WithGroup returns a new Handler that names the given group for any
// attributes it receives.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		out:      h.out,
		level:    h.level,
		useColor: h.useColor,
		attrs:    append(make([]slog.Attr, 0, len(h.attrs)), h.attrs...),
		groups:   append(append(make([]string, 0, len(h.groups)+1), h.groups...), name),
	}
}

// levelString returns a 5-character upper-case level name. DEBUG and ERROR
// occupy all 5 characters; INFO and WARN are padded so every level tag
// aligns in the same column.
func levelString(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "DEBUG"
	case l < slog.LevelWarn:
		return "INFO "
	case l < slog.LevelError:
		return "WARN "
	default:
		return "ERROR"
	}
}

// levelStyle returns the ANSI escape sequence for the level colour.
func levelStyle(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return gray
	case l < slog.LevelWarn:
		return cyan
	case l < slog.LevelError:
		return yellow
	default:
		return red
	}
}

// writeAttr appends a single key=value pair, quoting the value if it
// contains whitespace.
func writeAttr(b *strings.Builder, a slog.Attr) {
	b.WriteString(a.Key)
	b.WriteByte('=')
	val := a.Value.String()
	if strings.ContainsAny(val, " \t\n\r") {
		fmt.Fprintf(b, "%q", val)
	} else {
		b.WriteString(val)
	}
}
