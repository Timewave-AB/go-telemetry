package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// textHandler renders a slog.Record as a single human-readable line:
//
//	2026-05-24 14:02:11 [INFO] msg="..." key=value key2="value 2"
//
// Timestamps are always UTC. Attributes are sorted alphabetically per record.
// Groups become dotted key prefixes (group.subgroup.key).
type textHandler struct {
	level  slog.Level
	writer io.Writer
	mu     *sync.Mutex // shared across all derived handlers so writes don't interleave
	attrs  []groupedAttr
	groups []string
}

type groupedAttr struct {
	key   string // already group-prefixed
	value slog.Value
}

func (h *textHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	collected := make([]groupedAttr, 0, len(h.attrs)+r.NumAttrs())
	collected = append(collected, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		collected = appendAttr(collected, h.groups, a)
		return true
	})
	sort.Slice(collected, func(i, j int) bool { return collected[i].key < collected[j].key })

	var b strings.Builder
	b.WriteString(r.Time.UTC().Format("2006-01-02 15:04:05"))
	b.WriteString(" [")
	b.WriteString(levelName(r.Level))
	b.WriteString(`] msg=`)
	writeValue(&b, slog.StringValue(r.Message))
	for _, a := range collected {
		b.WriteByte(' ')
		b.WriteString(a.key)
		b.WriteByte('=')
		writeValue(&b, a.value)
	}
	b.WriteByte('\n')

	mu := h.lock()
	mu.Lock()
	defer mu.Unlock()
	_, err := io.WriteString(h.writer, b.String())
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	out := h.clone()
	for _, a := range attrs {
		out.attrs = appendAttr(out.attrs, h.groups, a)
	}
	return out
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	out := h.clone()
	out.groups = append(append([]string{}, h.groups...), name)
	return out
}

func (h *textHandler) clone() *textHandler {
	return &textHandler{
		level:  h.level,
		writer: h.writer,
		mu:     h.lock(),
		attrs:  append([]groupedAttr{}, h.attrs...),
		groups: append([]string{}, h.groups...),
	}
}

func (h *textHandler) lock() *sync.Mutex {
	if h.mu == nil {
		h.mu = &sync.Mutex{}
	}
	return h.mu
}

// appendAttr flattens groups into dotted prefixes and skips empty/zero attrs.
func appendAttr(dst []groupedAttr, groups []string, a slog.Attr) []groupedAttr {
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if a.Value.Kind() == slog.KindGroup {
		nested := append(groups, a.Key)
		for _, sub := range a.Value.Group() {
			dst = appendAttr(dst, nested, sub)
		}
		return dst
	}
	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + a.Key
	}
	return append(dst, groupedAttr{key: key, value: a.Value.Resolve()})
}

// writeValue prints strings (and string-ish kinds) quoted via %q,
// everything else via %v. Keeps the format unambiguous when values
// contain spaces or equals signs.
func writeValue(b *strings.Builder, v slog.Value) {
	switch v.Kind() {
	case slog.KindString:
		fmt.Fprintf(b, "%q", v.String())
	default:
		fmt.Fprintf(b, "%v", v.Any())
	}
}
