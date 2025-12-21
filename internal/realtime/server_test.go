package realtime

import (
	"context"
	"testing"
	"time"

	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/websocket"
	realtimepb "gitlab.com/spydotech-group/shared-entity/proto/realtime/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubHub struct {
	websocket.Hub
}

func TestReportRSVPEventPushesOntoHub(t *testing.T) {
	ch := make(chan models.EventRSVPEvent, 1)
	hub := &stubHub{
		Hub: websocket.Hub{
			EventRSVPEvents: ch,
		},
	}

	server := NewServer(&hub.Hub)
	now := time.Now().UTC()

	req := &realtimepb.ReportRSVPEventRequest{
		EventId:   "event-123",
		UserId:    "user-456",
		Status:    "going",
		Timestamp: timestamppb.New(now),
		Stats: &realtimepb.RSVPStats{
			GoingCount:      10,
			InterestedCount: 5,
			InvitedCount:    20,
		},
	}

	if _, err := server.ReportRSVPEvent(context.Background(), req); err != nil {
		t.Fatalf("ReportRSVPEvent returned error: %v", err)
	}

	select {
	case evt := <-ch:
		if evt.EventID != req.GetEventId() || evt.UserID != req.GetUserId() || string(evt.Status) != req.GetStatus() {
			t.Fatalf("unexpected event payload %+v", evt)
		}
		if evt.Stats.GoingCount != req.GetStats().GetGoingCount() {
			t.Fatalf("stats mismatch: %+v", evt.Stats)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event to be published onto hub channel")
	}
}
