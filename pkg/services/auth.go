package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-faster/errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/requestmeta"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/constants"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap"
)

// Auth-related constants
const (
	authCookieName       = "access_token"
	refreshTokenMaxAge   = 315360000                 // 10 years in seconds
	refreshTokenDuration = 10 * 365 * 24 * time.Hour // 10 years
)

func (a *apiService) AuthLogin(ctx context.Context, session *api.AuthAttemptSession) (*api.AuthLoginNoContent, error) {

	if !checkUserIsAllowed(a.cnf.JWT.AllowedUsers, session.UserName) {
		return nil, &apiError{code: http.StatusForbidden, err: errors.New("user not allowed")}
	}

	now := time.Now().UTC()

	jwtClaims := &types.JWTClaims{
		Name:      session.Name,
		UserName:  session.UserName,
		IsPremium: session.IsPremium,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(session.UserId, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.cnf.JWT.SessionTime)),
		}}

	sessionID := uuid.New()
	jwtClaims.SessionID = sessionID

	jwtToken, err := auth.Encode(a.cnf.JWT.Secret, jwtClaims)

	if err != nil {
		return nil, &apiError{err: err}
	}

	client, err := a.telegram.AuthClient(ctx, session.Session, 5)
	if err != nil {
		return nil, &apiError{err: err}
	}
	authorizations, err := a.telegram.ListAuthorizations(ctx, client)
	if err != nil {
		return nil, &apiError{err: err}
	}

	var dateCreated int32
	for _, authorization := range authorizations {
		if authorization.Current {
			dateCreated = authorization.DateCreated
			break
		}
	}

	refreshToken, err := generateToken(32)
	if err != nil {
		return nil, &apiError{err: err}
	}
	refreshTokenHash := hashToken(refreshToken)

	if err := a.repo.WithTx(ctx, func(txCtx context.Context) error {
		if _, err := a.repo.Users.GetByID(txCtx, session.UserId); err != nil {
			if !errors.Is(err, repositories.ErrNotFound) {
				return err
			}
			name := session.Name
			now := time.Now().UTC()
			user := &jetmodel.Users{
				UserID:    session.UserId,
				Name:      &name,
				UserName:  session.UserName,
				IsPremium: session.IsPremium,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := a.repo.Users.Create(txCtx, user); err != nil {
				return err
			}
		}

		if _, err := a.repo.Files.GetActiveByNameAndParent(txCtx, session.UserId, "root", nil); err != nil {
			if !errors.Is(err, repositories.ErrNotFound) {
				return err
			}

			now := time.Now().UTC()
			root := &jetmodel.Files{
				ID:        uuid.New(),
				Name:      "root",
				Type:      "folder",
				MimeType:  "drive/folder",
				UserID:    session.UserId,
				Status:    utils.Ptr(constants.FileStatusActive.String()),
				Encrypted: false,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := a.repo.Files.Create(txCtx, root); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		sessionRow := &jetmodel.Sessions{
			ID:               sessionID,
			UserID:           session.UserId,
			TgSession:        session.Session,
			RefreshTokenHash: &refreshTokenHash,
			SessionDate:      &dateCreated,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := a.repo.Sessions.Create(txCtx, sessionRow); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, &apiError{err: err}
	}

	if err := a.ensureDefaultPeriodicJobs(ctx, session.UserId); err != nil {
		return nil, err
	}

	setRefreshCookie(ctx, refreshToken)
	return &api.AuthLoginNoContent{
		SetCookie: setCookie(ctx, authCookieName, jwtToken, int(a.cnf.JWT.SessionTime.Seconds())),
	}, nil
}

func (a *apiService) AuthRefresh(ctx context.Context, params api.AuthRefreshParams) (*api.AuthRefreshNoContent, error) {
	refreshToken := params.RefreshToken
	if refreshToken == "" {
		return nil, &apiError{code: http.StatusUnauthorized, err: errors.New("missing refresh token")}
	}

	sessionRow, err := a.repo.Sessions.GetByRefreshTokenHash(ctx, hashToken(refreshToken))
	if err != nil {
		return nil, &apiError{code: http.StatusUnauthorized, err: errors.New("invalid refresh token")}
	}

	user, err := a.repo.Users.GetByID(ctx, sessionRow.UserID)
	if err != nil {
		return nil, &apiError{err: err}
	}
	name := user.UserName
	if user.Name != nil && *user.Name != "" {
		name = *user.Name
	}

	now := time.Now().UTC()
	jwtClaims := &types.JWTClaims{
		Name:      name,
		UserName:  user.UserName,
		IsPremium: user.IsPremium,
		SessionID: sessionRow.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.UserID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.cnf.JWT.SessionTime)),
		},
	}

	jwtToken, err := auth.Encode(a.cnf.JWT.Secret, jwtClaims)
	if err != nil {
		return nil, &apiError{err: err}
	}

	newRefreshToken, err := generateToken(32)
	if err != nil {
		return nil, &apiError{err: err}
	}
	if err := a.repo.Sessions.UpdateRefreshTokenHash(ctx, sessionRow.ID, hashToken(newRefreshToken)); err != nil {
		return nil, &apiError{err: err}
	}

	setRefreshCookie(ctx, newRefreshToken)

	return &api.AuthRefreshNoContent{SetCookie: setCookie(ctx, authCookieName, jwtToken, int(a.cnf.JWT.SessionTime.Seconds()))}, nil
}

func (a *apiService) AuthLogout(ctx context.Context) (*api.AuthLogoutNoContent, error) {
	authUser := auth.JWTUser(ctx)
	if err := a.repo.Sessions.Revoke(ctx, authUser.SessionID); err != nil {
		return nil, &apiError{err: err}
	}
	userId, err := strconv.ParseInt(authUser.Subject, 10, 64)
	if err != nil {
		return nil, &apiError{err: fmt.Errorf("invalid user subject: %w", err)}
	}
	a.cache.Delete(ctx, cache.KeySessionID(authUser.SessionID.String()), cache.KeyUserSessions(userId))
	_ = a.cache.DeletePattern(ctx, cache.KeyAPIKeyAuthPattern())
	clearRefreshCookie(ctx)
	client, err := a.telegram.AuthClient(ctx, authUser.TgSession, 5)
	if err != nil {
		logging.FromContext(ctx).Warn("failed to initialize telegram client for logout", zap.Error(err))
		return &api.AuthLogoutNoContent{SetCookie: setCookie(ctx, authCookieName, "", -1)}, nil
	}
	if err := a.telegram.LogOut(ctx, client); err != nil {
		logging.FromContext(ctx).Warn("telegram logout failed", zap.Error(err))
		return &api.AuthLogoutNoContent{SetCookie: setCookie(ctx, authCookieName, "", -1)}, nil
	}

	return &api.AuthLogoutNoContent{SetCookie: setCookie(ctx, authCookieName, "", -1)}, nil
}

func (a *apiService) AuthSession(ctx context.Context) (api.AuthSessionRes, error) {
	claims := auth.JWTUser(ctx)
	if claims == nil {
		return &api.AuthSessionNoContent{}, nil
	}

	claims.TgSession = ""

	now := time.Now().UTC()

	userId, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return &api.AuthSessionNoContent{}, nil
	}
	user, err := a.repo.Users.GetByID(ctx, userId)
	if err != nil {
		return nil, &apiError{err: err}
	}

	name := user.UserName
	if user.Name != nil && *user.Name != "" {
		name = *user.Name
	}

	newExpires := now.Add(a.cnf.JWT.SessionTime)

	session := api.Session{
		Name:      name,
		UserName:  user.UserName,
		IsPremium: user.IsPremium,
		UserId:    userId,
		SessionId: api.UUID(claims.SessionID),
		Expires:   newExpires}

	response := &api.SessionHeaders{Response: session}
	if auth.Source(ctx) == auth.AuthSourceAPIKey {
		return response, nil
	}

	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(newExpires)
	jweToken, err := auth.Encode(a.cnf.JWT.Secret, claims)
	if err != nil {
		return &api.AuthSessionNoContent{}, nil
	}
	response.SetCookie = api.NewOptString(setCookie(ctx, authCookieName, jweToken, int(a.cnf.JWT.SessionTime.Seconds())))
	return response, nil
}

func ip4toInt(ipv4Address net.IP) int64 {
	IPv4Int := big.NewInt(0)
	IPv4Int.SetBytes(ipv4Address.To4())
	return IPv4Int.Int64()
}

func pack32BinaryIP4(ip4Address string) []byte {
	ipv4Decimal := ip4toInt(net.ParseIP(ip4Address))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(ipv4Decimal))
	return buf.Bytes()
}

func generateTgSession(dcId int, authKey []byte, port int) string {

	dcMaps := map[int]string{
		1: "149.154.175.53",
		2: "149.154.167.51",
		3: "149.154.175.100",
		4: "149.154.167.91",
		5: "91.108.56.130",
	}

	dcIDByte := byte(dcId)
	serverAddressBytes := pack32BinaryIP4(dcMaps[dcId])
	portByte := make([]byte, 2)
	binary.BigEndian.PutUint16(portByte, uint16(port))

	packet := make([]byte, 0)
	packet = append(packet, dcIDByte)
	packet = append(packet, serverAddressBytes...)
	packet = append(packet, portByte...)
	packet = append(packet, authKey...)

	base64Encoded := base64.URLEncoding.EncodeToString(packet)
	return "1" + base64Encoded
}

func checkUserIsAllowed(allowedUsers []string, userName string) bool {
	found := false
	if len(allowedUsers) > 0 {
		if slices.Contains(allowedUsers, userName) {
			found = true
		}
	} else {
		found = true
	}
	return found
}

func prepareSession(user *TelegramUser, data *types.SessionData) *api.AuthAttemptSession {
	sessionString := generateTgSession(data.Data.DC, data.Data.AuthKey, 443)
	session := &api.AuthAttemptSession{
		Session:   sessionString,
		UserId:    user.ID,
		UserName:  user.Username,
		Name:      fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		IsPremium: user.Premium,
	}
	return session
}

func setCookie(ctx context.Context, name, value string, maxAge int) string {
	cookie := http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   requestmeta.IsSecure(ctx),
	}
	return cookie.String()
}

func setRefreshCookie(ctx context.Context, token string) {
	requestmeta.AddSetCookie(ctx, (&http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		MaxAge:   refreshTokenMaxAge,
		Expires:  time.Now().Add(refreshTokenDuration),
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   requestmeta.IsSecure(ctx),
	}).String())
}

func clearRefreshCookie(ctx context.Context) {
	requestmeta.AddSetCookie(ctx, (&http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   requestmeta.IsSecure(ctx),
	}).String())
}
