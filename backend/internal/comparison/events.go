package comparison

import (
	"sync"
	"time"
)

// Event is one change published to subscribers (the UI's event stream).
type Event struct {
	Seq        int64
	Comparison *View
	DeletedID  string
}

// bus fans events out to subscribers. Changes are coalesced: a comparison that changes many
// times within a tick is published once, and live comparisons are republished every second so
// their timers and metrics move.
type bus struct {
	mu    sync.Mutex
	seq   int64
	subs  map[chan Event]struct{}
	dirty map[string]*comparison
}

func (b *bus) init() {
	b.subs = map[chan Event]struct{}{}
	b.dirty = map[string]*comparison{}
}

// Subscribe returns a channel of events and a function to stop receiving them.
func (s *Service) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 1024)
	s.events.mu.Lock()
	s.events.subs[ch] = struct{}{}
	s.events.mu.Unlock()
	return ch, func() {
		s.events.mu.Lock()
		if _, ok := s.events.subs[ch]; ok {
			delete(s.events.subs, ch)
			close(ch)
		}
		s.events.mu.Unlock()
	}
}

// changed marks a comparison for publishing on the next tick.
func (s *Service) changed(c *comparison) {
	s.events.mu.Lock()
	s.events.dirty[c.id] = c
	s.events.mu.Unlock()
}

func (s *Service) changedSide(sd *side) {
	s.mu.Lock()
	c := s.all[sd.comparisonID]
	s.mu.Unlock()
	if c != nil {
		s.changed(c)
	}
}

func (s *Service) publishLoop() {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for i := 0; ; i++ {
		<-t.C
		s.events.mu.Lock()
		batch := s.events.dirty
		s.events.dirty = map[string]*comparison{}
		s.events.mu.Unlock()
		if i%4 == 0 {
			// Every second: running comparisons, for their timers and live metrics.
			s.mu.Lock()
			for id, c := range s.all {
				for _, sd := range c.sides {
					if !terminalStatuses[sd.status] {
						batch[id] = c
						break
					}
				}
			}
			s.mu.Unlock()
		}
		for _, c := range batch {
			v := s.view(c)
			s.publish(Event{Comparison: &v})
		}
	}
}

func (s *Service) publish(e Event) {
	s.events.mu.Lock()
	defer s.events.mu.Unlock()
	s.events.seq++
	e.Seq = s.events.seq
	for ch := range s.events.subs {
		select {
		case ch <- e:
		default:
			// A subscriber that cannot keep up is dropped; it reconnects and gets the current state.
			delete(s.events.subs, ch)
			close(ch)
		}
	}
}
