package api

import (
	"context"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

func (h *Handler) GetBotManager(ctx context.Context, params gen.GetBotManagerParams) (gen.GetBotManagerRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	manager, err := h.Bots.Manager(ctx, userID, params.BotId)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.BotManagerStatus{ID: manager.ID, Username: manager.Username, CanManageBots: true}, nil
}

func (h *Handler) CheckManagedBotUsername(ctx context.Context, req *gen.BotUsernameRequest) (gen.CheckManagedBotUsernameRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Bots.CheckManagedUsername(ctx, userID, req.Username); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.BotUsernameAvailability{Available: true}, nil
}

func (h *Handler) CreateManagedBot(ctx context.Context, req *gen.ManagedBotCreateRequest, _ gen.CreateManagedBotParams) (gen.CreateManagedBotRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil || h.Jobs == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	bot, recovered, err := h.Bots.CreateManaged(ctx, userID, req.ManagerBotId, req.Name, req.Username)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := &gen.ManagedBotCreateResponse{Bot: botSummary(bot), Recovered: recovered}
	if !bot.Enabled {
		jobID, err := h.Jobs.InsertBotProvision(ctx, userID, []int64{bot.BotID})
		if err != nil {
			return nil, mapServiceError(err)
		}
		if len(jobID) > 0 {
			response.JobId = gen.NewOptString(jobID[0])
		}
	}
	return response, nil
}
