package api

import (
	"context"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

func (h *Handler) CreateEventStreamTicket(ctx context.Context) (gen.CreateEventStreamTicketRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h == nil || h.Events == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	ticket, err := h.Events.IssueTicket(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.EventStreamTicket{Ticket: ticket.Value, ExpiresAt: ticket.ExpiresAt}, nil
}
