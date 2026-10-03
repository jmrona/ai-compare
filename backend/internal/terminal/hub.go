package terminal

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// maxBuffer is how much recent output a hub keeps for viewers who connect later.
const maxBuffer = 2 << 20

// Hub owns a side's terminal for its whole life: it keeps the output (so a reopened tab or a
// second tab sees the same screen), fans it out to every connected browser, and sends their
// keystrokes and size changes to the container.
type Hub struct {
	mu     sync.Mutex
	buf    []byte
	subs   map[chan []byte]struct{}
	input  io.Writer
	resize func(cols, rows uint)
	closed bool
	log    *slog.Logger
	// The latest size a viewer asked for. Viewers often connect before the container exists,
	// so it is kept and applied once the container starts.
	cols, rows uint
	rec        *Recorder
	// inputs are the times the user typed, at most one per second; they tell human wait time apart.
	inputs []time.Time
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}, log: log}
}

// Connect wires the hub to an attached container: input goes to it and resize changes its TTY size.
func (h *Hub) Connect(input io.Writer, resize func(cols, rows uint)) {
	h.mu.Lock()
	h.input, h.resize = input, resize
	h.mu.Unlock()
}

// Pump copies container output into the hub until r ends.
func (h *Hub) Pump(r io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Record starts writing the output to an asciicast recording.
func (h *Hub) Record(r *Recorder) {
	h.mu.Lock()
	h.rec = r
	h.mu.Unlock()
}

// Preload puts output in the buffer without recording or sending it: the screen of a side
// reattached after a restart, or of a finished one opened from the history.
func (h *Hub) Preload(p []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, p...)
	if over := len(h.buf) - maxBuffer; over > 0 {
		h.buf = append([]byte(nil), h.buf[over:]...)
	}
}

// Inputs returns the times the user typed into the terminal.
func (h *Hub) Inputs() []time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]time.Time(nil), h.inputs...)
}

// SetInputs restores the input times of a reattached side.
func (h *Hub) SetInputs(ts []time.Time) {
	h.mu.Lock()
	h.inputs = append([]time.Time(nil), ts...)
	h.mu.Unlock()
}

// Write records output and sends it to every viewer. Also used for ai-compare's own messages.
func (h *Hub) Write(p []byte) (int, error) {
	chunk := append([]byte(nil), p...)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rec != nil {
		h.rec.output(time.Now(), chunk)
	}
	h.buf = append(h.buf, chunk...)
	if over := len(h.buf) - maxBuffer; over > 0 {
		h.buf = append([]byte(nil), h.buf[over:]...)
	}
	for ch := range h.subs {
		select {
		case ch <- chunk:
		default:
			// A viewer that cannot keep up is dropped; reconnecting replays the buffer.
			delete(h.subs, ch)
			close(ch)
		}
	}
	return len(p), nil
}

// Note writes a dim ai-compare message line.
func (h *Hub) Note(msg string) {
	h.Write([]byte("\x1b[90m[ai-compare] " + msg + "\x1b[0m\r\n"))
}

// Close disconnects input; viewers keep the final screen.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.input = nil
	if h.rec != nil {
		h.rec.Close()
		h.rec = nil
	}
	for ch := range h.subs {
		close(ch)
	}
	h.subs = map[chan []byte]struct{}{}
}

// Output is the recorded output (the last maxBuffer bytes).
func (h *Hub) Output() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]byte(nil), h.buf...)
}

func (h *Hub) subscribe() ([]byte, chan []byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	snapshot := append([]byte(nil), h.buf...)
	if h.closed {
		return snapshot, nil, false
	}
	ch := make(chan []byte, 512)
	h.subs[ch] = struct{}{}
	return snapshot, ch, true
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *Hub) send(data []byte) {
	h.mu.Lock()
	w := h.input
	if w != nil {
		if now := time.Now(); len(h.inputs) == 0 || now.Sub(h.inputs[len(h.inputs)-1]) >= time.Second {
			h.inputs = append(h.inputs, now)
		}
	}
	h.mu.Unlock()
	if w != nil {
		w.Write(data)
	}
}

func (h *Hub) setSize(cols, rows uint) {
	if cols == 0 || rows == 0 {
		return
	}
	h.mu.Lock()
	h.cols, h.rows = cols, rows
	resize := h.resize
	if h.rec != nil {
		h.rec.resize(time.Now(), cols, rows)
	}
	h.mu.Unlock()
	if resize != nil {
		resize(cols, rows)
	}
}

// Size is the latest size a viewer asked for, or the fallback when nobody has connected yet.
func (h *Hub) Size(fallbackCols, fallbackRows uint) (cols, rows uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cols == 0 {
		return fallbackCols, fallbackRows
	}
	return h.cols, h.rows
}

// ApplySize sends the latest known size to the container (call it once the container runs).
func (h *Hub) ApplySize() {
	h.mu.Lock()
	cols, rows, resize := h.cols, h.rows, h.resize
	h.mu.Unlock()
	if resize != nil && cols > 0 {
		resize(cols, rows)
	}
}

// ServeHTTP upgrades to a WebSocket (same origin only), replays the buffer and then streams.
// Same protocol as Bridge: binary output and keystrokes, JSON text frames for resize.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	snapshot, ch, live := h.subscribe()
	if len(snapshot) > 0 {
		if err := conn.Write(ctx, websocket.MessageBinary, snapshot); err != nil {
			return
		}
	}
	if !live {
		conn.Close(websocket.StatusNormalClosure, "the session has ended")
		return
	}
	defer h.unsubscribe(ch)

	go func() {
		defer cancel()
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageText {
				var c control
				if json.Unmarshal(data, &c) == nil && c.Type == "resize" {
					h.setSize(c.Cols, c.Rows)
				}
				continue
			}
			h.send(data)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-ch:
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "the session has ended")
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, chunk); err != nil {
				return
			}
		}
	}
}
