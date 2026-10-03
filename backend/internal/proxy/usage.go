package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Usage is the token count of one request, split the way providers bill it.
// Input excludes cached tokens, so the four fields add up to the total.
type Usage struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Output     int64 `json:"output"`
	// Reported is false when the response carried no usage (the UI shows "not reported", never 0).
	Reported bool `json:"reported"`
}

func (u Usage) Total() int64 { return u.Input + u.CacheRead + u.CacheWrite + u.Output }

// PromptTokens is everything sent to the model in this request, used for long-context pricing.
func (u Usage) PromptTokens() int64 { return u.Input + u.CacheRead + u.CacheWrite }

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
	u.Output += o.Output
	u.Reported = u.Reported || o.Reported
}

// rawUsage covers the usage objects of the three APIs the CLIs use:
//   - OpenAI Chat Completions: prompt_tokens (includes cached), completion_tokens, prompt_tokens_details.cached_tokens
//   - OpenAI Responses:        input_tokens (includes cached), output_tokens, input_tokens_details.cached_tokens
//   - Anthropic Messages:      input_tokens (excludes cache), output_tokens, cache_read_input_tokens, cache_creation_input_tokens
type rawUsage struct {
	PromptTokens        *int64 `json:"prompt_tokens"`
	CompletionTokens    *int64 `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`

	InputTokens        *int64 `json:"input_tokens"`
	OutputTokens       *int64 `json:"output_tokens"`
	InputTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`

	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens"`
}

func (r *rawUsage) normalise() (Usage, bool) {
	if r == nil {
		return Usage{}, false
	}
	switch {
	case r.PromptTokens != nil || r.CompletionTokens != nil: // Chat Completions
		u := Usage{Input: val(r.PromptTokens), Output: val(r.CompletionTokens), Reported: true}
		if r.PromptTokensDetails != nil {
			u.CacheRead = r.PromptTokensDetails.CachedTokens
			u.Input -= u.CacheRead
		}
		return u, true
	case r.CacheReadInputTokens != nil || r.CacheCreationInputTokens != nil: // Anthropic
		return Usage{
			Input:      val(r.InputTokens),
			CacheRead:  val(r.CacheReadInputTokens),
			CacheWrite: val(r.CacheCreationInputTokens),
			Output:     val(r.OutputTokens),
			Reported:   true,
		}, true
	case r.InputTokens != nil || r.OutputTokens != nil: // Responses (or Anthropic without cache fields)
		u := Usage{Input: val(r.InputTokens), Output: val(r.OutputTokens), Reported: true}
		if r.InputTokensDetails != nil {
			u.CacheRead = r.InputTokensDetails.CachedTokens
			u.Input -= u.CacheRead
		}
		return u, true
	}
	return Usage{}, false
}

func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// event is the subset of a JSON body or SSE event that can carry usage.
type event struct {
	Type     string    `json:"type"`
	Usage    *rawUsage `json:"usage"`
	Response *struct {
		Usage *rawUsage `json:"usage"`
	} `json:"response"`
	Message *struct {
		Usage *rawUsage `json:"usage"`
	} `json:"message"`
}

// usageFromJSON reads the usage of a non-streamed response.
func usageFromJSON(body []byte) Usage {
	var e event
	if json.Unmarshal(body, &e) != nil {
		return Usage{}
	}
	if u, ok := e.Usage.normalise(); ok {
		return u
	}
	if e.Response != nil {
		if u, ok := e.Response.Usage.normalise(); ok {
			return u
		}
	}
	return Usage{}
}

// sseUsage accumulates usage from a server-sent event stream as it passes through.
type sseUsage struct {
	usage Usage
	// Anthropic sends input and cache counts in message_start and the output count in message_delta.
	anthropicStart Usage
}

func (s *sseUsage) data(payload []byte) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || payload[0] != '{' {
		return // e.g. "[DONE]"
	}
	var e event
	if json.Unmarshal(payload, &e) != nil {
		return
	}
	switch e.Type {
	case "response.completed", "response.incomplete", "response.failed": // Responses API
		if e.Response != nil {
			if u, ok := e.Response.Usage.normalise(); ok {
				s.usage = u
			}
		}
	case "message_start": // Anthropic
		if e.Message != nil {
			if u, ok := e.Message.Usage.normalise(); ok {
				s.anthropicStart = u
				s.usage = u
			}
		}
	case "message_delta": // Anthropic: cumulative output tokens
		if e.Usage != nil && e.Usage.OutputTokens != nil {
			u := s.anthropicStart
			u.Output = *e.Usage.OutputTokens
			u.Reported = true
			s.usage = u
		}
	default: // Chat Completions: the last chunk carries usage when include_usage is on
		if u, ok := e.Usage.normalise(); ok {
			s.usage = u
		}
	}
}

// meter wraps a response body: the client reads it unchanged while usage is extracted.
type meter struct {
	body      io.ReadCloser
	streaming bool
	buf       bytes.Buffer // whole body for JSON, pending partial line for SSE
	sse       sseUsage
	done      func(Usage, error)
	finished  bool
}

// maxJSONBody caps how much of a non-streamed body is kept for parsing.
const maxJSONBody = 8 << 20

func newMeter(body io.ReadCloser, contentType string, done func(Usage, error)) *meter {
	return &meter{body: body, streaming: strings.Contains(contentType, "text/event-stream"), done: done}
}

func (m *meter) Read(p []byte) (int, error) {
	n, err := m.body.Read(p)
	if n > 0 {
		if m.streaming {
			m.buf.Write(p[:n])
			m.consumeLines()
		} else if m.buf.Len() < maxJSONBody {
			m.buf.Write(p[:n])
		}
	}
	if err != nil {
		m.finish(err)
	}
	return n, err
}

func (m *meter) consumeLines() {
	for {
		line, err := m.buf.ReadBytes('\n')
		if err != nil {
			// Incomplete line: keep it for the next read.
			rest := append([]byte(nil), line...)
			m.buf.Reset()
			m.buf.Write(rest)
			return
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			m.sse.data(line[len("data:"):])
		}
	}
}

func (m *meter) Close() error {
	err := m.body.Close()
	m.finish(io.ErrUnexpectedEOF)
	return err
}

func (m *meter) finish(readErr error) {
	if m.finished {
		return
	}
	m.finished = true
	var u Usage
	if m.streaming {
		if m.buf.Len() > 0 {
			sc := bufio.NewScanner(&m.buf)
			for sc.Scan() {
				if line := sc.Bytes(); bytes.HasPrefix(line, []byte("data:")) {
					m.sse.data(line[len("data:"):])
				}
			}
		}
		u = m.sse.usage
	} else {
		u = usageFromJSON(m.buf.Bytes())
	}
	if readErr == io.EOF {
		readErr = nil
	}
	m.done(u, readErr)
}
