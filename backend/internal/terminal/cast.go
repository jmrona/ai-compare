package terminal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

// Recorder writes a terminal session in asciicast v2 format (https://docs.asciinema.org): a JSON
// header line, then one [seconds, "o" | "r", data] line per output chunk or resize. It is what
// the history replays with its original timing.
type Recorder struct {
	mu    sync.Mutex
	f     *os.File
	w     *bufio.Writer
	start time.Time
	// carry holds the end of a chunk that split a multi-byte character.
	carry []byte
}

// NewRecorder creates the file, or appends to it when it already exists (a side reattached after
// a restart keeps recording into the same file).
func NewRecorder(path string, cols, rows uint, start time.Time) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	r := &Recorder{f: f, w: bufio.NewWriter(f), start: start}
	if os.IsNotExist(statErr) {
		header, _ := json.Marshal(map[string]any{
			"version": 2, "width": cols, "height": rows, "timestamp": start.Unix(),
			"env": map[string]string{"TERM": "xterm-256color"},
		})
		r.w.Write(append(header, '\n'))
	}
	return r, nil
}

func (r *Recorder) output(at time.Time, p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	data := append(r.carry, p...)
	// Keep an incomplete trailing UTF-8 sequence for the next chunk.
	cut := len(data)
	for i := len(data) - 1; i >= 0 && i >= len(data)-utf8.UTFMax; i-- {
		if utf8.RuneStart(data[i]) {
			if !utf8.FullRune(data[i:]) {
				cut = i
			}
			break
		}
	}
	r.carry = append([]byte(nil), data[cut:]...)
	r.event(at, "o", string(data[:cut]))
}

func (r *Recorder) resize(at time.Time, cols, rows uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.event(at, "r", fmt.Sprintf("%dx%d", cols, rows))
	}
}

// event must be called with r.mu held.
func (r *Recorder) event(at time.Time, kind, data string) {
	if data == "" {
		return
	}
	line, _ := json.Marshal([]any{at.Sub(r.start).Seconds(), kind, data})
	r.w.Write(append(line, '\n'))
	r.w.Flush()
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	if len(r.carry) > 0 {
		r.event(time.Now(), "o", string(r.carry))
	}
	r.w.Flush()
	err := r.f.Close()
	r.f = nil
	return err
}
