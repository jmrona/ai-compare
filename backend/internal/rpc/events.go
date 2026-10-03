package rpc

import (
	"context"

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
	// Headers now, so the browser knows the stream is open even when nothing is running.
	if err := stream.SendHeaders(); err != nil {
		return err
	}
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
