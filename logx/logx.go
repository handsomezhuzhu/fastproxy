// Package logx provides structured access/event logging with an in-memory
// ring buffer (served to the admin panel) and a rotating JSONL file.
package logx

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level is a log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ParseLevel maps a settings string to a Level (default info).
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// Entry is one structured log line (connection events and system messages).
type Entry struct {
	Time       string `json:"time"`
	Level      string `json:"level"`
	Port       int    `json:"port,omitempty"`
	Account    string `json:"account,omitempty"`
	Client     string `json:"client,omitempty"`
	Proto      string `json:"proto,omitempty"`  // http | socks5
	Target     string `json:"target,omitempty"` // CONNECT / request target
	Status     string `json:"status,omitempty"` // ok | auth-failed | upstream-error | ...
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	DurationMS int64  `json:"duration_ms"`
	Msg        string `json:"msg,omitempty"`
}

// Persist is the database side of the logger (SQLite in production).
type Persist interface {
	InsertLog(Entry) error
	QueryLogs(Filter) ([]Entry, error)
}

// Logger keeps recent entries, persists to a database when configured, and
// optionally appends JSONL to a file (when no database is attached).
type Logger struct {
	mu     sync.Mutex
	buf    []Entry
	max    int
	level  Level
	access bool
	file   *os.File
	path   string
	size   int64

	store   Persist
	ch      chan Entry
	done    chan struct{}
	dropped atomic.Int64
}

const maxFileSize = 8 << 20 // 8 MiB per generation, one .1 backup kept

// New opens (or creates) the JSONL file. An empty path disables file output.
// Attach a database with SetPersist; the file is then only a fallback.
func New(path string, buffer int, level Level, access bool) *Logger {
	l := &Logger{max: buffer, level: level, access: access, path: path}
	l.applyDefaults()
	if path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			l.file = f
			if st, err := f.Stat(); err == nil {
				l.size = st.Size()
			}
		}
	}
	return l
}

// SetPersist attaches a database and starts the async writer.
func (l *Logger) SetPersist(p Persist) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.store != nil || p == nil {
		return
	}
	l.store = p
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.ch = make(chan Entry, 2048)
	l.done = make(chan struct{})
	go l.writer()
}

// writer drains the channel in small batches to keep the relay path hot.
func (l *Logger) writer() {
	defer close(l.done)
	batch := make([]Entry, 0, 100)
	store := l.store
	if store == nil {
		return
	}
	flush := func() {
		for _, e := range batch {
			if err := store.InsertLog(e); err != nil {
				l.dropped.Add(1)
			}
		}
		batch = batch[:0]
	}
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 100 {
				flush()
			}
		case <-tick.C:
			if len(batch) > 0 {
				flush()
			}
		}
	}
}

func (l *Logger) applyDefaults() {
	if l.max <= 0 {
		l.max = 1000
	}
	if l.buf == nil {
		l.buf = make([]Entry, 0, l.max)
	}
}

// SetLevel updates the minimum emitted level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}

// SetBuffer updates the ring buffer size (trims immediately).
func (l *Logger) SetBuffer(max int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if max <= 0 {
		max = 1000
	}
	l.max = max
	if len(l.buf) > max {
		l.buf = append([]Entry(nil), l.buf[len(l.buf)-max:]...)
	}
}

// SetAccess enables/disables access (per-connection) entries.
func (l *Logger) SetAccess(enabled bool) {
	l.mu.Lock()
	l.access = enabled
	l.mu.Unlock()
}

// Emit records one entry (filtered by level) into the ring, the database or
// the fallback file.
func (l *Logger) Emit(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lv := ParseLevel(e.Level)
	if lv < l.level {
		return
	}
	if e.Level == "" {
		e.Level = lv.String()
	}
	if e.Time == "" {
		e.Time = time.Now().Format(time.RFC3339)
	}
	l.buf = append(l.buf, e)
	if len(l.buf) > l.max {
		over := len(l.buf) - l.max
		l.buf = append([]Entry(nil), l.buf[over:]...)
	}
	if l.store != nil && l.ch != nil {
		select {
		case l.ch <- e:
		default:
			l.dropped.Add(1) // database writer is saturated; keep forwarding hot
		}
		return
	}
	if l.file == nil {
		return
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	n, err := l.file.Write(append(line, '\n'))
	if err != nil {
		return
	}
	l.size += int64(n)
	if l.size > maxFileSize {
		l.rotateLocked()
	}
}

func (l *Logger) rotateLocked() {
	l.file.Close()
	_ = os.Rename(l.path, l.path+".1")
	if f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		l.file = f
		l.size = 0
	}
}

// Close flushes the async writer and closes the file.
func (l *Logger) Close() {
	l.mu.Lock()
	ch, done := l.ch, l.done
	l.ch = nil
	l.mu.Unlock()
	if ch != nil {
		close(ch)
		<-done
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// Filter selects entries for Recent/CountLogs/QueryLogs.
type Filter struct {
	Port    int
	Account string
	Level   string // minimum level
	Limit   int
	Offset  int
}

// Recent returns entries newest-first. The database is authoritative when
// attached; the ring covers the window before the writer flushes.
func (l *Logger) Recent(f Filter) []Entry {
	l.mu.Lock()
	store := l.store
	l.mu.Unlock()
	if store != nil {
		if out, err := store.QueryLogs(f); err == nil {
			return out
		}
	}
	return l.recentRing(f)
}

func (l *Logger) recentRing(f Filter) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	minLevel := ParseLevel(f.Level)
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	out := make([]Entry, 0, limit)
	for i := len(l.buf) - 1; i >= 0 && len(out) < limit; i-- {
		e := l.buf[i]
		if f.Port != 0 && e.Port != f.Port {
			continue
		}
		if f.Account != "" && e.Account != f.Account {
			continue
		}
		if ParseLevel(e.Level) < minLevel {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Infof emits an info system message.
func (l *Logger) Infof(format string, args ...any) {
	l.Emit(Entry{Level: "info", Msg: fmt.Sprintf(format, args...)})
}

// Warnf emits a warn system message.
func (l *Logger) Warnf(format string, args ...any) {
	l.Emit(Entry{Level: "warn", Msg: fmt.Sprintf(format, args...)})
}

// Errorf emits an error system message.
func (l *Logger) Errorf(format string, args ...any) {
	l.Emit(Entry{Level: "error", Msg: fmt.Sprintf(format, args...)})
}
