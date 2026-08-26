package api

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/fileops"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
)

func (h *Handler) TelegramLoginStart(ctx context.Context, req *gen.TelegramLoginStartRequest, params gen.TelegramLoginStartParams) (gen.TelegramLoginStartRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	flow, err := h.Auth.StartLogin(ctx, req.PhoneNumber)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := loginFlowResponse(flow)
	return &response, nil
}

func (h *Handler) TelegramQRLoginStart(ctx context.Context, params gen.TelegramQRLoginStartParams) (gen.TelegramQRLoginStartRes, error) {
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	flow, err := h.Auth.StartQR(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := qrLoginFlowResponse(flow)
	return &response, nil
}

func (h *Handler) TelegramQRLoginPoll(ctx context.Context, req *gen.TelegramQRLoginPollRequest, params gen.TelegramQRLoginPollParams) (gen.TelegramQRLoginPollRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.PollQR(ctx, googleUUID(req.FlowId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	if result.QRFlow != nil {
		response := qrLoginFlowResponse(result.QRFlow)
		return &response, nil
	}
	response := tokenPairResponse(result.Tokens)
	return &response, nil
}
func (h *Handler) TelegramLoginVerifyCode(ctx context.Context, req *gen.TelegramCodeVerifyRequest, params gen.TelegramLoginVerifyCodeParams) (gen.TelegramLoginVerifyCodeRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.VerifyCode(ctx, googleUUID(req.FlowId), req.Code)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if result.Flow != nil {
		response := loginFlowResponse(result.Flow)
		return &response, nil
	}
	response := tokenPairResponse(result.Tokens)
	return &response, nil
}

func (h *Handler) TelegramLoginVerifyPassword(ctx context.Context, req *gen.TelegramPasswordVerifyRequest, params gen.TelegramLoginVerifyPasswordParams) (gen.TelegramLoginVerifyPasswordRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	result, err := h.Auth.VerifyPassword(ctx, googleUUID(req.FlowId), req.Password)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := tokenPairResponse(result.Tokens)
	return &response, nil
}

func (h *Handler) RefreshSession(ctx context.Context, req *gen.RefreshTokenRequest) (gen.RefreshSessionRes, error) {
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	tokens, err := h.Auth.Refresh(ctx, req.RefreshToken)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := tokenPairResponse(tokens)
	return &response, nil
}

func (h *Handler) LogoutSession(ctx context.Context) (gen.LogoutSessionRes, error) {
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil, mapServiceError(ErrUnauthenticated)
	}
	if err := h.Auth.Logout(ctx, identity); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.LogoutSessionNoContent{}, nil
}

func (h *Handler) GetCurrentUser(ctx context.Context) (gen.GetCurrentUserRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	user, err := h.Auth.GetUser(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := userProfile(user)
	return &response, nil
}

func (h *Handler) GetProfilePhoto(ctx context.Context) (gen.GetProfilePhotoRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.TelegramAccount == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	photo, found, err := h.TelegramAccount.ProfilePhoto(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if !found {
		return &gen.GetProfilePhotoNoContent{}, nil
	}
	return &gen.GetProfilePhotoOKHeaders{
		CacheControl:       "private, max-age=86400, must-revalidate",
		ContentDisposition: `inline; filename="profile.jpeg"`,
		ContentLength:      int64(len(photo.Content)),
		Etag:               gen.ETag(fmt.Sprintf("\"%d\"", photo.PhotoID)),
		Response:           gen.GetProfilePhotoOK{Data: bytes.NewReader(photo.Content)},
	}, nil
}
func (h *Handler) CreateApiKey(ctx context.Context, req *gen.ApiKeyCreateRequest, params gen.CreateApiKeyParams) (gen.CreateApiKeyRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var expires *time.Time
	if value, ok := req.ExpiresAt.Get(); ok {
		expires = &value
	}
	created, err := h.Auth.CreateAPIKey(ctx, userID, req.Name, expires)
	if err != nil {
		return nil, mapServiceError(err)
	}
	rowID, _ := dbtypes.GoogleUUID(created.Row.ID)
	response := gen.ApiKeyCreated{
		ID: apiUUID(rowID), Name: created.Row.Name, Secret: created.Secret,
		CreatedAt: created.Row.CreatedAt.Time,
	}
	if created.Row.ExpiresAt.Valid {
		response.ExpiresAt = gen.NewOptDateTime(created.Row.ExpiresAt.Time)
	}
	return &response, nil
}

func (h *Handler) ListApiKeys(ctx context.Context, params gen.ListApiKeysParams) (gen.ListApiKeysRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor datedUUIDCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(authn.ErrInvalidInput)
	}
	input := authn.ListAPIKeysInput{UserID: userID, Limit: params.Limit.Or(100)}
	if cursor.ID != uuid.Nil {
		input.AfterID, input.AfterCreatedAt = &cursor.ID, &cursor.CreatedAt
	}
	rows, err := h.Auth.ListAPIKeys(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.ApiKeySummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, apiKeySummary(row))
	}
	response := gen.ListApiKeysOK{Items: items}
	if len(rows) == int(input.Limit) && len(rows) > 0 {
		lastID, _ := dbtypes.GoogleUUID(rows[len(rows)-1].ID)
		response.NextCursor = encodeCursor(datedUUIDCursor{CreatedAt: rows[len(rows)-1].CreatedAt.Time, ID: lastID})
	}
	return &response, nil
}

func (h *Handler) RevokeApiKey(ctx context.Context, params gen.RevokeApiKeyParams) (gen.RevokeApiKeyRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Auth == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Auth.RevokeAPIKey(ctx, userID, googleUUID(params.ApiKeyId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.RevokeApiKeyNoContent{}, nil
}

func (h *Handler) ListSessions(ctx context.Context, params gen.ListSessionsParams) (gen.ListSessionsRes, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok || h.Auth == nil {
		return nil, mapServiceError(authn.ErrSessionNotFound)
	}
	var cursor datedUUIDCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(authn.ErrInvalidInput)
	}
	input := authn.ListSessionsInput{UserID: identity.UserID, Limit: params.Limit.Or(100)}
	if cursor.ID != uuid.Nil {
		input.AfterID, input.AfterCreatedAt = &cursor.ID, &cursor.CreatedAt
	}
	rows, err := h.Auth.ListSessions(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.SessionSummary, 0, len(rows))
	for _, row := range rows {
		sessionID, ok := dbtypes.GoogleUUID(row.ID)
		if !ok {
			return nil, mapServiceError(authn.ErrSessionNotFound)
		}
		item := gen.SessionSummary{
			ID: apiUUID(sessionID), Current: sessionID == identity.SessionID,
			CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
		}
		if row.LastUsedAt.Valid {
			item.LastUsedAt = gen.NewOptDateTime(row.LastUsedAt.Time)
		}
		if row.RevokedAt.Valid {
			item.RevokedAt = gen.NewOptDateTime(row.RevokedAt.Time)
		}
		items = append(items, item)
	}
	response := gen.ListSessionsOK{Items: items}
	if len(rows) == int(input.Limit) && len(rows) > 0 {
		lastID, _ := dbtypes.GoogleUUID(rows[len(rows)-1].ID)
		response.NextCursor = encodeCursor(datedUUIDCursor{CreatedAt: rows[len(rows)-1].CreatedAt.Time, ID: lastID})
	}
	return &response, nil
}

func (h *Handler) RevokeSession(ctx context.Context, params gen.RevokeSessionParams) (gen.RevokeSessionRes, error) {
	identity, ok := IdentityFromContext(ctx)
	if !ok || h.Auth == nil {
		return nil, mapServiceError(authn.ErrSessionNotFound)
	}
	if err := h.Auth.RevokeSession(ctx, identity.UserID, googleUUID(params.SessionId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.RevokeSessionNoContent{}, nil
}

func (h *Handler) CreateBots(ctx context.Context, req *gen.BotCreateRequest, params gen.CreateBotsParams) (gen.CreateBotsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil || h.Jobs == nil || req == nil || len(req.Tokens) == 0 {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	response := gen.BotCreateResponse{Bots: []gen.BotSummary{}, FailedIndexes: []int32{}}
	tokens := make([]string, 0, len(req.Tokens))
	seen := make(map[int64]struct{}, len(req.Tokens))
	for index, raw := range req.Tokens {
		token := strings.TrimSpace(raw)
		botID, parseErr := bots.TokenBotID(token)
		if parseErr != nil {
			response.FailedIndexes = append(response.FailedIndexes, int32(index))
			continue
		}
		if _, exists := seen[botID]; exists {
			response.FailedIndexes = append(response.FailedIndexes, int32(index))
			continue
		}
		seen[botID] = struct{}{}
		tokens = append(tokens, token)
	}
	rows, insertErr := h.Bots.InsertPending(ctx, userID, tokens)
	if insertErr != nil {
		return nil, mapServiceError(insertErr)
	}
	botIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		response.Bots = append(response.Bots, botSummary(row))
		botIDs = append(botIDs, row.BotID)
	}
	if len(botIDs) > 0 {
		jobID, jobErr := h.Jobs.InsertBotProvision(ctx, userID, botIDs)
		if jobErr != nil {
			return nil, mapServiceError(jobErr)
		}
		if jobID != "" {
			response.JobId = gen.NewOptString(jobID)
		}
	}
	return &response, nil
}

func (h *Handler) ListBots(ctx context.Context, params gen.ListBotsParams) (gen.ListBotsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor datedInt64Cursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(bots.ErrInvalidInput)
	}
	input := bots.ListInput{UserID: userID, Limit: params.Limit.Or(100)}
	if cursor.ID != 0 {
		input.AfterCreatedAt, input.AfterBotID = &cursor.CreatedAt, &cursor.ID
	}
	rows, err := h.Bots.List(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.BotSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, botSummary(row))
	}
	response := gen.ListBotsOK{Items: items}
	if len(rows) == int(input.Limit) && len(rows) > 0 {
		last := rows[len(rows)-1]
		response.NextCursor = encodeCursor(datedInt64Cursor{CreatedAt: last.CreatedAt.Time, ID: last.BotID})
	}
	return &response, nil
}

func (h *Handler) DeleteBot(ctx context.Context, params gen.DeleteBotParams) (gen.DeleteBotRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Bots == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Bots.Delete(ctx, userID, params.BotId); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.DeleteBotNoContent{}, nil
}

func (h *Handler) DiscoverChannels(ctx context.Context) (gen.DiscoverChannelsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.TelegramAccount == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	remote, err := h.TelegramAccount.DiscoverChannels(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.DiscoveredChannel, 0, len(remote))
	for _, channel := range remote {
		items = append(items, gen.DiscoveredChannel{ID: channel.ID, Name: channel.Name})
	}
	response := gen.DiscoverChannelsOKApplicationJSON(items)
	return &response, nil
}

func (h *Handler) SyncChannels(ctx context.Context, params gen.SyncChannelsParams) (gen.SyncChannelsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.TelegramAccount == nil || h.Channels == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	discovered, err := h.TelegramAccount.DiscoverChannels(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}
	remote := make([]channels.RemoteChannel, 0, len(discovered))
	for _, channel := range discovered {
		remote = append(remote, channels.RemoteChannel{ID: channel.ID, Name: channel.Name})
	}
	rows, err := h.Channels.Sync(ctx, userID, remote)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.ChannelSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, channelSummary(row))
	}
	response := gen.SyncChannelsOKApplicationJSON(items)
	return &response, nil
}
func (h *Handler) CreateChannel(ctx context.Context, req *gen.ChannelCreateRequest, params gen.CreateChannelParams) (gen.CreateChannelRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Channels == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	row, err := h.Channels.Create(ctx, userID, req.Name.Or(""), req.Selected.Or(false))
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := channelSummary(row)
	return &response, nil
}

func (h *Handler) ListChannels(ctx context.Context, params gen.ListChannelsParams) (gen.ListChannelsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Channels == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor datedInt64Cursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(channels.ErrInvalidChannel)
	}
	input := channels.ListInput{UserID: userID, Limit: params.Limit.Or(100)}
	if cursor.ID != 0 {
		input.AfterCreatedAt, input.AfterChannelID = &cursor.CreatedAt, &cursor.ID
	}
	rows, err := h.Channels.List(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.ChannelSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, channelSummary(row))
	}
	response := gen.ListChannelsOK{Items: items}
	if len(rows) == int(input.Limit) && len(rows) > 0 {
		last := rows[len(rows)-1]
		response.NextCursor = encodeCursor(datedInt64Cursor{CreatedAt: last.CreatedAt.Time, ID: last.ChannelID})
	}
	return &response, nil
}

func (h *Handler) SelectChannel(ctx context.Context, params gen.SelectChannelParams) (gen.SelectChannelRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Channels == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	row, err := h.Channels.Select(ctx, userID, params.ChannelId)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := channelSummary(row)
	return &response, nil
}

func (h *Handler) DeleteChannel(ctx context.Context, params gen.DeleteChannelParams) (gen.DeleteChannelRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Channels == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Channels.Delete(ctx, userID, params.ChannelId); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.DeleteChannelNoContent{}, nil
}

func (h *Handler) CopyFile(ctx context.Context, req *gen.FileCopyRequest, params gen.CopyFileParams) (gen.CopyFileRes, error) {
	if h.FileOps == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var name *string
	if value, ok := req.Name.Get(); ok {
		name = &value
	}
	policy := sqlcgen.NameConflictPolicyFail
	if value, ok := req.ConflictPolicy.Get(); ok {
		policy = sqlcgen.NameConflictPolicy(value)
	}
	sourceAccess, err := h.resolveAuthenticatedFileAccess(ctx, googleUUID(params.FileId), false)
	if err != nil {
		return nil, mapServiceError(err)
	}
	parentID := optionalGoogleUUID(req.ParentId)
	if parentID == nil {
		if !sourceAccess.Owned {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	} else {
		destinationAccess, err := h.resolveAuthenticatedFileAccess(ctx, *parentID, true)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if destinationAccess.OwnerID != sourceAccess.OwnerID {
			return nil, mapServiceError(shares.ErrForbidden)
		}
	}
	file, err := h.FileOps.Copy(ctx, fileops.CopyInput{
		UserID: sourceAccess.OwnerID, FileID: googleUUID(params.FileId), ParentID: parentID, Name: name,
		ConflictPolicy: policy,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(file)
	if err != nil {
		return nil, mapServiceError(err)
	}
	fileID := googleUUID(entry.ID)
	return &gen.CopyFileCreatedHeaders{
		Etag: generationETag(file.Generation), Location: gen.URI(url.URL{Path: "/v1/files/" + fileID.String()}),
		Response: entry,
	}, nil
}

func (h *Handler) PurgeFile(ctx context.Context, params gen.PurgeFileParams) (gen.PurgeFileRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.FileOps == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.FileOps.Purge(ctx, userID, googleUUID(params.FileId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.PurgeFileNoContent{}, nil
}

func (h *Handler) CleanTrash(ctx context.Context) (gen.CleanTrashRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.FileOps == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if _, err := h.FileOps.CleanTrash(ctx, userID); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.CleanTrashNoContent{}, nil
}

func (h *Handler) CreateShare(ctx context.Context, req *gen.ShareCreateRequest, params gen.CreateShareParams) (gen.CreateShareRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var password *string
	if value, ok := req.Password.Get(); ok {
		password = &value
	}
	var expires *time.Time
	if value, ok := req.ExpiresAt.Get(); ok {
		expires = &value
	}
	var maxDownloads *int64
	if value, ok := req.MaxDownloads.Get(); ok {
		maxDownloads = &value
	}
	permission := sqlcgen.SharePermissionRead
	if value, ok := req.Permission.Get(); ok {
		permission = sqlcgen.SharePermission(value)
	}
	created, err := h.Shares.Create(ctx, shares.CreateInput{
		OwnerID: userID, FileID: googleUUID(params.FileId), Password: password,
		ExpiresAt: expires, MaxDownloads: maxDownloads, Permission: permission,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := shareCreated(created)
	return &response, nil
}

func (h *Handler) ListFileShares(ctx context.Context, params gen.ListFileSharesParams) (gen.ListFileSharesRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	var cursor datedUUIDCursor
	if err := decodeCursor(params.Cursor, &cursor); err != nil {
		return nil, mapServiceError(shares.ErrInvalidInput)
	}
	input := shares.ListInput{OwnerID: userID, FileID: googleUUID(params.FileId), Limit: params.Limit.Or(100)}
	if cursor.ID != uuid.Nil {
		input.AfterCreatedAt, input.AfterID = &cursor.CreatedAt, &cursor.ID
	}
	rows, err := h.Shares.List(ctx, input)
	if err != nil {
		return nil, mapServiceError(err)
	}
	items := make([]gen.ShareSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, shareSummary(row))
	}
	response := gen.ListFileSharesOK{Items: items}
	if len(rows) == int(input.Limit) && len(rows) > 0 {
		last := rows[len(rows)-1]
		lastID, _ := dbtypes.GoogleUUID(last.ID)
		response.NextCursor = encodeCursor(datedUUIDCursor{CreatedAt: last.CreatedAt.Time, ID: lastID})
	}
	return &response, nil
}

func (h *Handler) RevokeShare(ctx context.Context, params gen.RevokeShareParams) (gen.RevokeShareRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	if err := h.Shares.Revoke(ctx, userID, googleUUID(params.ShareId)); err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.RevokeShareNoContent{}, nil
}

func (h *Handler) GetPublicShare(ctx context.Context, params gen.GetPublicShareParams) (gen.GetPublicShareRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	resolved, err := h.Shares.Resolve(ctx, params.Token, params.XSharePassword.Or(""))
	if err != nil {
		return nil, mapServiceError(err)
	}
	entry, err := fileEntry(resolved.File)
	if err != nil {
		return nil, mapServiceError(err)
	}
	shareID, _ := dbtypes.GoogleUUID(resolved.Share.ID)
	response := gen.PublicShare{
		ID: apiUUID(shareID), File: entry, PasswordProtected: resolved.Share.PasswordHash.Valid,
		Permission: gen.SharePermission(resolved.Share.Permission),
	}
	if resolved.Share.ExpiresAt.Valid {
		response.ExpiresAt = gen.NewOptDateTime(resolved.Share.ExpiresAt.Time)
	}
	return &response, nil
}

func (h *Handler) HeadPublicShare(ctx context.Context, params gen.HeadPublicShareParams) (gen.HeadPublicShareRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	resolved, err := h.Shares.Resolve(ctx, params.Token, params.XSharePassword.Or(""))
	if err != nil {
		return nil, mapServiceError(err)
	}
	file := resolved.File
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid {
		return nil, mapServiceError(transfer.ErrInvalidDownload)
	}
	return &gen.HeadPublicShareOK{
		AcceptRanges: gen.HeadPublicShareOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func (h *Handler) HeadPublicShareLegacy(ctx context.Context, params gen.HeadPublicShareLegacyParams) (gen.HeadPublicShareLegacyRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	resolved, err := h.Shares.Resolve(ctx, params.Token, params.XSharePassword.Or(""))
	if err != nil {
		return nil, mapServiceError(err)
	}
	file := resolved.File
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid {
		return nil, mapServiceError(transfer.ErrInvalidDownload)
	}
	return &gen.HeadPublicShareLegacyOK{
		AcceptRanges: gen.HeadPublicShareLegacyOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func (h *Handler) HeadPublicShareFile(ctx context.Context, params gen.HeadPublicShareFileParams) (gen.HeadPublicShareFileRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	resolved, err := h.Shares.ResolveFile(ctx, params.Token, params.XSharePassword.Or(""), googleUUID(params.FileId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	file := resolved.File
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid || file.Size.Int64 < 0 {
		return nil, mapServiceError(transfer.ErrInvalidDownload)
	}
	return &gen.HeadPublicShareFileOK{
		AcceptRanges: gen.HeadPublicShareFileOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func (h *Handler) HeadPublicShareFileLegacy(ctx context.Context, params gen.HeadPublicShareFileLegacyParams) (gen.HeadPublicShareFileLegacyRes, error) {
	if h.Shares == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	resolved, err := h.Shares.ResolveFile(ctx, params.Token, params.XSharePassword.Or(""), googleUUID(params.FileId))
	if err != nil {
		return nil, mapServiceError(err)
	}
	file := resolved.File
	if file.Kind != sqlcgen.FileKindFile || !file.Size.Valid || file.Size.Int64 < 0 {
		return nil, mapServiceError(transfer.ErrInvalidDownload)
	}
	return &gen.HeadPublicShareFileLegacyOK{
		AcceptRanges: gen.HeadPublicShareFileLegacyOKAcceptRanges("bytes"), ContentDisposition: contentDisposition(file.Name, false),
		ContentLength: file.Size.Int64, Etag: contentETag(file), LastModified: file.ModTime.Time,
	}, nil
}

func loginFlowResponse(flow *authn.FlowResult) gen.TelegramLoginStartResponse {
	return gen.TelegramLoginStartResponse{FlowId: apiUUID(flow.ID), ExpiresAt: flow.ExpiresAt, PasswordRequired: flow.PasswordRequired}
}

func qrLoginFlowResponse(flow *authn.QRFlowResult) gen.TelegramQRLoginResponse {
	state := gen.TelegramQRLoginStatePending
	if flow.PasswordRequired {
		state = gen.TelegramQRLoginStatePasswordRequired
	}
	response := gen.TelegramQRLoginResponse{
		FlowId: apiUUID(flow.ID), ExpiresAt: flow.ExpiresAt, State: state,
	}
	if flow.QRURL != "" {
		response.QrUrl = gen.NewOptString(flow.QRURL)
	}
	if !flow.QRExpiresAt.IsZero() {
		response.QrExpiresAt = gen.NewOptDateTime(flow.QRExpiresAt)
	}
	return response
}
func tokenPairResponse(tokens *authn.TokenPair) gen.TokenPair {
	return gen.TokenPair{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, TokenType: gen.TokenPairTokenTypeBearer, ExpiresIn: tokens.ExpiresIn}
}

func userProfile(row *sqlcgen.User) gen.UserProfile {
	out := gen.UserProfile{
		UserId: row.UserID, Premium: row.Premium, Role: gen.UserRole(row.Role),
		Capabilities: authn.Capabilities(row.Role), CreatedAt: row.CreatedAt.Time,
	}
	if row.DisplayName.Valid {
		out.DisplayName = gen.NewOptString(row.DisplayName.String)
	}
	if row.Username.Valid {
		out.Username = gen.NewOptString(row.Username.String)
	}
	return out
}

func apiKeySummary(row *sqlcgen.ApiKey) gen.ApiKeySummary {
	id, _ := dbtypes.GoogleUUID(row.ID)
	out := gen.ApiKeySummary{ID: apiUUID(id), Name: row.Name, CreatedAt: row.CreatedAt.Time}
	if row.LastUsedAt.Valid {
		out.LastUsedAt = gen.NewOptDateTime(row.LastUsedAt.Time)
	}
	if row.ExpiresAt.Valid {
		out.ExpiresAt = gen.NewOptDateTime(row.ExpiresAt.Time)
	}
	if row.RevokedAt.Valid {
		out.RevokedAt = gen.NewOptDateTime(row.RevokedAt.Time)
	}
	return out
}

func botSummary(row *sqlcgen.Bot) gen.BotSummary {
	out := gen.BotSummary{ID: row.BotID, Enabled: row.Enabled, CreatedAt: row.CreatedAt.Time}
	if row.Username.Valid {
		out.Username = gen.NewOptString(row.Username.String)
	}
	return out
}

func channelSummary(row *sqlcgen.Channel) gen.ChannelSummary {
	return gen.ChannelSummary{
		ID: row.ChannelID, Name: row.Name, Selected: row.Selected,
		Health: gen.ChannelHealth(row.Health), CreatedAt: row.CreatedAt.Time,
	}
}

func shareCreated(created *shares.Created) gen.ShareCreated {
	id, _ := dbtypes.GoogleUUID(created.Row.ID)
	fileID, _ := dbtypes.GoogleUUID(created.Row.FileID)
	out := gen.ShareCreated{
		ID: apiUUID(id), FileId: apiUUID(fileID), Token: created.Token, PublicUrl: gen.URI(created.PublicURL),
		PasswordProtected: created.Row.PasswordHash.Valid, Permission: gen.SharePermission(created.Row.Permission), CreatedAt: created.Row.CreatedAt.Time,
	}
	if created.Row.ExpiresAt.Valid {
		out.ExpiresAt = gen.NewOptDateTime(created.Row.ExpiresAt.Time)
	}
	if created.Row.MaxDownloads.Valid {
		out.MaxDownloads = gen.NewOptInt64(created.Row.MaxDownloads.Int64)
	}
	return out
}

func shareSummary(row *sqlcgen.FileShare) gen.ShareSummary {
	id, _ := dbtypes.GoogleUUID(row.ID)
	fileID, _ := dbtypes.GoogleUUID(row.FileID)
	out := gen.ShareSummary{
		ID: apiUUID(id), FileId: apiUUID(fileID), PasswordProtected: row.PasswordHash.Valid,
		DownloadCount: row.DownloadCount, Permission: gen.SharePermission(row.Permission), CreatedAt: row.CreatedAt.Time,
	}
	if row.ExpiresAt.Valid {
		out.ExpiresAt = gen.NewOptDateTime(row.ExpiresAt.Time)
	}
	if row.MaxDownloads.Valid {
		out.MaxDownloads = gen.NewOptInt64(row.MaxDownloads.Int64)
	}
	if row.RevokedAt.Valid {
		out.RevokedAt = gen.NewOptDateTime(row.RevokedAt.Time)
	}
	return out
}

type datedUUIDCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type datedInt64Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}
