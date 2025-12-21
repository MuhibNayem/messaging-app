package realtime

import (
	"context"
	"log"

	"gitlab.com/spydotech-group/shared-entity/models"
	"messaging-app/internal/websocket"
	realtimepb "gitlab.com/spydotech-group/shared-entity/proto/realtime/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements the RealtimeService RPCs.
type Server struct {
	realtimepb.UnimplementedRealtimeServiceServer

	hub *websocket.Hub
}

func NewServer(hub *websocket.Hub) *Server {
	return &Server{hub: hub}
}

func (s *Server) ReportRSVPEvent(ctx context.Context, req *realtimepb.ReportRSVPEventRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}

	event := models.EventRSVPEvent{
		EventID:   req.GetEventId(),
		UserID:    req.GetUserId(),
		Status:    models.RSVPStatus(req.GetStatus()),
		Timestamp: req.GetTimestamp().AsTime(),
	}

	if stats := req.GetStats(); stats != nil {
		event.Stats = models.EventStats{
			GoingCount:      stats.GetGoingCount(),
			InterestedCount: stats.GetInterestedCount(),
			InvitedCount:    stats.GetInvitedCount(),
		}
	}

	select {
	case s.hub.EventRSVPEvents <- event:
	default:
		log.Printf("realtime: dropping RSVP event for %s due to full channel", event.EventID)
	}

	return &emptypb.Empty{}, nil
}
