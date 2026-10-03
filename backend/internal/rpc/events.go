package rpc

import (
	"context"
	"time"

	"connectrpc.com/connect/v2"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/gen/aicompare/v1/aicomparev1connect"
)

type eventService struct {
	svc *comparison.Service
}

// Watch sends the live comparisons first, then every change until the client goes away.
func (s *eventService) Watch(ctx context.Context, _ *v1.WatchRequest, stream aicomparev1connect.EventServiceWatchServerStream) error {
	// Subscribe before taking the snapshot, so nothing between the two is lost.
	events, unsubscribe := s.svc.Subscribe()
	defer unsubscribe()
	// An empty message first, so the client knows the stream is open even when nothing is
	// running (in connect-go v2 RC1 SendHeaders does not flush on the server).
	if err := stream.Send(&v1.WatchResponse{}); err != nil {
		return err
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for _, v := range s.svc.List() {
		if v.Live() {
			if err := stream.Send(&v1.WatchResponse{Event: &v1.WatchResponse_Comparison{Comparison: ComparisonToProto(v)}}); err != nil {
				return err
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			// Keeps idle connections from being closed by proxies along the way.
			if err := stream.Send(&v1.WatchResponse{}); err != nil {
				return err
			}
		case e, ok := <-events:
			if !ok {
				// Dropped for being too slow: the client reconnects and starts from the current state.
				return connect.NewError(connect.CodeUnavailable, "the event stream fell behind; reconnect")
			}
			res := &v1.WatchResponse{Seq: e.Seq}
			if e.DeletedID != "" {
				res.Event = &v1.WatchResponse_DeletedId{DeletedId: e.DeletedID}
			} else {
				res.Event = &v1.WatchResponse_Comparison{Comparison: ComparisonToProto(*e.Comparison)}
			}
			if err := stream.Send(res); err != nil {
				return err
			}
		}
	}
}
