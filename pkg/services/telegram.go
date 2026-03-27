package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/pool"
	tgc "github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/types"
)

type TelegramUser struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	Premium   bool
}

type TelegramAuthorization struct {
	DateCreated int32
	Current     bool
	AppName     string
	Country     string
	OfficialApp bool
}

const (
	TelegramOpStream    = "stream"
	TelegramOpUpload    = "upload"
	qrRenewBeforeExpiry = 10 * time.Second
)

type ChannelManager interface {
	CurrentChannel(ctx context.Context, userID int64) (int64, error)
	BotTokens(ctx context.Context, userID int64) ([]string, error)
	ChannelLimitReached(channelID int64) bool
	CreateNewChannel(ctx context.Context, newChannelName string, userID int64, setDefault bool) (int64, error)
	AddBotsToChannel(ctx context.Context, userID int64, channelID int64, botsTokens []string, save bool) error
}

type TelegramClient interface {
	API() *tg.Client
	Run(ctx context.Context, f func(ctx context.Context) error) error
	RandInt64() (int64, error)
	Self(ctx context.Context) (*TelegramUser, error)
	QRLogin(ctx context.Context, loggedIn qrlogin.LoggedIn, onToken func(ctx context.Context, tokenURL string) error) (*TelegramUser, error)
	SendCode(ctx context.Context, phoneNo string) (string, error)
	SignIn(ctx context.Context, phoneNo, phoneCode, phoneCodeHash string) (*TelegramUser, error)
	Password(ctx context.Context, password string) (*TelegramUser, error)
}

type UploadPool interface {
	Default(ctx context.Context) *tg.Client
	Close()
}

type TelegramService interface {
	AuthClient(ctx context.Context, sessionStr string, retries int) (TelegramClient, error)
	BotClient(ctx context.Context, token string, retries int) (TelegramClient, error)
	SelectBotToken(ctx context.Context, operation string, userID int64, tokens []string) (string, int, error)
	NewQRLogin() (tg.UpdateDispatcher, qrlogin.LoggedIn)
	NoAuthClient(ctx context.Context, dispatcher tg.UpdateDispatcher, storage session.Storage) (TelegramClient, error)
	RunWithAuth(ctx context.Context, client TelegramClient, token string, f func(ctx context.Context) error) error
	LogOut(ctx context.Context, client TelegramClient) error
	ListAuthorizations(ctx context.Context, client TelegramClient) ([]TelegramAuthorization, error)
	DeleteChannel(ctx context.Context, client TelegramClient, channelID int64) (storage.PeerKey, error)
	SyncDialogs(ctx context.Context, client TelegramClient, peerStorage storage.PeerStorage) error
	GetProfilePhoto(ctx context.Context, client TelegramClient) ([]byte, int64, bool, error)
	NewUploadPool(ctx context.Context, client TelegramClient, size int64, retries int) (UploadPool, error)
	GetMessages(ctx context.Context, client TelegramClient, ids []int, channelID int64) ([]tg.MessageClass, error)
	GetParts(ctx context.Context, client TelegramClient, channelID int64, fileParts []api.Part, encrypted bool) ([]types.Part, error)
	CopyFileParts(ctx context.Context, client TelegramClient, sourceChannelID int64, destinationChannelID int64, sourceParts []api.Part) ([]api.Part, error)
	UploadPart(ctx context.Context, apiClient *tg.Client, channelID int64, partName string, fileStream io.Reader, fileSize int64, threads int) (int, int64, error)
	ChannelByID(ctx context.Context, client TelegramClient, channelID int64) (*tg.InputChannel, error)
	ChannelByIDRaw(ctx context.Context, api *tg.Client, channelID int64) (*tg.InputChannel, error)
	GetChannelFull(ctx context.Context, client TelegramClient, channelID int64) (*tg.Channel, error)
	GetMediaContent(ctx context.Context, client TelegramClient, location tg.InputFileLocationClass) (*bytes.Buffer, error)
	IsNoDefaultChannelError(err error) bool
	IsPasswordAuthNeeded(err error) bool
	IsSessionPasswordNeeded(err error) bool
	IsAuthKeyUnregistered(err error) bool
}

type telegramService struct {
	repo        *repositories.Repositories
	cache       cache.Cacher
	cnf         *config.TGConfig
	botSelector tgc.BotSelector
}

type telegramClient struct {
	client *telegram.Client
}

type telegramUploadPool struct {
	pool pool.Pool
}

func NewTelegramService(repo *repositories.Repositories, cache cache.Cacher, cnf *config.TGConfig, botSelector tgc.BotSelector) TelegramService {
	return &telegramService{repo: repo, cache: cache, cnf: cnf, botSelector: botSelector}
}

func (g *telegramService) middlewares(ctx context.Context, retries int) []telegram.Middleware {
	return tgc.NewMiddleware(g.cnf,
		tgc.WithFloodWait(),
		tgc.WithRecovery(ctx),
		tgc.WithRetry(retries),
		tgc.WithRateLimit(),
	)
}

func (g *telegramService) AuthClient(ctx context.Context, sessionStr string, retries int) (TelegramClient, error) {
	client, err := tgc.AuthClient(ctx, g.cnf, sessionStr, g.middlewares(ctx, retries)...)
	if err != nil {
		return nil, err
	}
	return &telegramClient{client: client}, nil
}

func (g *telegramService) BotClient(ctx context.Context, token string, retries int) (TelegramClient, error) {
	client, err := tgc.BotClient(ctx, g.repo.KV, g.cache, g.cnf, token, g.middlewares(ctx, retries)...)
	if err != nil {
		return nil, err
	}
	return &telegramClient{client: client}, nil
}

func (g *telegramService) SelectBotToken(ctx context.Context, operation string, userID int64, tokens []string) (string, int, error) {
	if g.botSelector == nil {
		return "", 0, fmt.Errorf("bot selector not configured")
	}

	var op tgc.BotOp
	switch operation {
	case TelegramOpStream:
		op = tgc.BotOpStream
	case TelegramOpUpload:
		op = tgc.BotOpUpload
	default:
		return "", 0, fmt.Errorf("unknown bot operation: %s", operation)
	}

	return g.botSelector.Next(ctx, op, userID, tokens)
}

func (g *telegramService) NewQRLogin() (tg.UpdateDispatcher, qrlogin.LoggedIn) {
	dispatcher := tg.NewUpdateDispatcher()
	return dispatcher, qrlogin.OnLoginToken(dispatcher)
}

func (g *telegramService) NoAuthClient(ctx context.Context, dispatcher tg.UpdateDispatcher, storage session.Storage) (TelegramClient, error) {
	client, err := tgc.NoAuthClient(ctx, g.cnf, dispatcher, storage)
	if err != nil {
		return nil, err
	}
	return &telegramClient{client: client}, nil
}

func (g *telegramService) RunWithAuth(ctx context.Context, client TelegramClient, token string, f func(ctx context.Context) error) error {
	rawClient, err := unwrapClient(client)
	if err != nil {
		return err
	}
	return tgc.RunWithAuth(ctx, rawClient, token, f)
}

func (g *telegramService) LogOut(ctx context.Context, client TelegramClient) error {
	return client.Run(ctx, func(ctx context.Context) error {
		_, err := client.API().AuthLogOut(ctx)
		return err
	})
}

func (g *telegramService) ListAuthorizations(ctx context.Context, client TelegramClient) ([]TelegramAuthorization, error) {
	out := make([]TelegramAuthorization, 0)
	err := client.Run(ctx, func(ctx context.Context) error {
		auths, err := client.API().AccountGetAuthorizations(ctx)
		if err != nil {
			return err
		}

		out = make([]TelegramAuthorization, 0, len(auths.Authorizations))
		for _, authorization := range auths.Authorizations {
			out = append(out, TelegramAuthorization{
				DateCreated: int32(authorization.DateCreated),
				Current:     authorization.Current,
				AppName:     authorization.AppName,
				Country:     authorization.Country,
				OfficialApp: authorization.OfficialApp,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (g *telegramService) DeleteChannel(ctx context.Context, client TelegramClient, channelID int64) (storage.PeerKey, error) {
	var out storage.PeerKey
	err := client.Run(ctx, func(ctx context.Context) error {
		channel, err := tgc.GetChannelFull(ctx, client.API(), channelID)
		if err != nil {
			return err
		}

		if _, err := client.API().ChannelsDeleteChannel(ctx, channel.AsInput()); err != nil {
			return err
		}

		peer := storage.Peer{}
		peer.FromChat(channel)
		out = storage.KeyFromPeer(peer)
		return nil
	})

	return out, err
}

func (g *telegramService) SyncDialogs(ctx context.Context, client TelegramClient, peerStorage storage.PeerStorage) error {
	collector := storage.CollectPeers(peerStorage)
	return client.Run(ctx, func(ctx context.Context) error {
		return collector.Dialogs(ctx, query.GetDialogs(client.API()).Iter())
	})
}

func (g *telegramService) GetProfilePhoto(ctx context.Context, client TelegramClient) ([]byte, int64, bool, error) {
	rawClient, err := unwrapClient(client)
	if err != nil {
		return nil, 0, false, err
	}

	var (
		content []byte
		photoID int64
		found   bool
	)

	err = client.Run(ctx, func(ctx context.Context) error {
		self, err := rawClient.Self(ctx)
		if err != nil {
			return err
		}

		if self.Photo == nil {
			return nil
		}

		photo, ok := self.Photo.AsNotEmpty()
		if !ok {
			return nil
		}

		location := &tg.InputPeerPhotoFileLocation{Big: false, Peer: self.AsInputPeer(), PhotoID: photo.PhotoID}
		buff, err := tgc.GetMediaContent(ctx, client.API(), location)
		if err != nil {
			return err
		}

		content = buff.Bytes()
		photoID = photo.PhotoID
		found = true
		return nil
	})
	if err != nil {
		return nil, 0, false, err
	}

	return content, photoID, found, nil
}

func (g *telegramService) NewUploadPool(ctx context.Context, client TelegramClient, size int64, retries int) (UploadPool, error) {
	rawClient, err := unwrapClient(client)
	if err != nil {
		return nil, err
	}
	return &telegramUploadPool{pool: pool.NewPool(rawClient, size, g.middlewares(ctx, retries)...)}, nil
}

func (g *telegramService) GetMessages(ctx context.Context, client TelegramClient, ids []int, channelID int64) ([]tg.MessageClass, error) {
	return tgc.GetMessages(ctx, client.API(), ids, channelID)
}

func (g *telegramService) GetParts(ctx context.Context, client TelegramClient, channelID int64, fileParts []api.Part, encrypted bool) ([]types.Part, error) {
	messages, err := tgc.GetMessages(ctx, client.API(), toPartIDs(fileParts), channelID)
	if err != nil {
		return nil, err
	}

	parts := make([]types.Part, 0, len(messages))
	for i, message := range messages {
		item, ok := message.(*tg.Message)
		if !ok {
			continue
		}

		media, ok := item.Media.(*tg.MessageMediaDocument)
		if !ok {
			continue
		}

		document, ok := media.Document.(*tg.Document)
		if !ok {
			continue
		}

		part := types.Part{
			ID:   int64(fileParts[i].ID),
			Size: document.Size,
			Salt: fileParts[i].Salt.Value,
		}
		if encrypted {
			decryptedSize, err := crypt.DecryptedSize(document.Size)
			if err != nil {
				return nil, err
			}
			part.DecryptedSize = decryptedSize
		}

		parts = append(parts, part)
	}

	return parts, nil
}

func (g *telegramService) CopyFileParts(ctx context.Context, client TelegramClient, sourceChannelID int64, destinationChannelID int64, sourceParts []api.Part) ([]api.Part, error) {
	messages, err := tgc.GetMessages(ctx, client.API(), toPartIDs(sourceParts), sourceChannelID)
	if err != nil {
		return nil, err
	}

	channel, err := tgc.ChannelByID(ctx, client.API(), destinationChannelID)
	if err != nil {
		return nil, err
	}

	out := make([]api.Part, 0, len(messages))
	for i, msg := range messages {
		item, ok := msg.(*tg.Message)
		if !ok {
			continue
		}

		media, ok := item.Media.(*tg.MessageMediaDocument)
		if !ok {
			continue
		}

		document, ok := media.Document.(*tg.Document)
		if !ok {
			continue
		}

		randomID, err := client.RandInt64()
		if err != nil {
			return nil, err
		}

		request := tg.MessagesSendMediaRequest{
			Silent:   true,
			Peer:     &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
			Media:    &tg.InputMediaDocument{ID: document.AsInput()},
			RandomID: randomID,
		}

		res, err := client.API().MessagesSendMedia(ctx, &request)
		if err != nil {
			return nil, err
		}

		updates, ok := res.(*tg.Updates)
		if !ok {
			return nil, fmt.Errorf("unexpected send media response %T", res)
		}

		var copied *tg.Message
		for _, update := range updates.Updates {
			channelMsg, ok := update.(*tg.UpdateNewChannelMessage)
			if ok {
				copied, _ = channelMsg.Message.(*tg.Message)
				break
			}
		}
		if copied == nil {
			return nil, fmt.Errorf("copied message not found")
		}

		part := api.Part{ID: copied.ID}
		if i < len(sourceParts) && sourceParts[i].Salt.Value != "" {
			part.Salt = api.NewOptString(sourceParts[i].Salt.Value)
		}
		out = append(out, part)
	}

	return out, nil
}

func (g *telegramService) UploadPart(ctx context.Context, apiClient *tg.Client, channelID int64, partName string, fileStream io.Reader, fileSize int64, threads int) (int, int64, error) {
	channel, err := tgc.ChannelByID(ctx, apiClient, channelID)
	if err != nil {
		return 0, 0, err
	}

	u := uploader.NewUploader(apiClient).WithThreads(threads).WithPartSize(512 * 1024)
	upload, err := u.Upload(ctx, uploader.NewUpload(partName, fileStream, fileSize))
	if err != nil {
		return 0, 0, err
	}

	document := message.UploadedDocument(upload).Filename(partName).ForceFile(true)
	sender := message.NewSender(apiClient)
	target := sender.To(&tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash})

	res, err := target.Media(ctx, document)
	if err != nil {
		return 0, 0, err
	}

	updates, ok := res.(*tg.Updates)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected upload response %T", res)
	}

	for _, update := range updates.Updates {
		channelMsg, ok := update.(*tg.UpdateNewChannelMessage)
		if !ok {
			continue
		}

		msg, ok := channelMsg.Message.AsNotEmpty()
		if !ok {
			continue
		}

		tgMessage, ok := msg.(*tg.Message)
		if !ok {
			continue
		}

		doc, ok := messageDocument(tgMessage)
		if !ok {
			return 0, 0, fmt.Errorf("upload failed: missing document")
		}

		return tgMessage.ID, doc.Size, nil
	}

	return 0, 0, fmt.Errorf("upload failed: invalid message response")
}

func (g *telegramService) ChannelByID(ctx context.Context, client TelegramClient, channelID int64) (*tg.InputChannel, error) {
	return tgc.ChannelByID(ctx, client.API(), channelID)
}

func (g *telegramService) ChannelByIDRaw(ctx context.Context, api *tg.Client, channelID int64) (*tg.InputChannel, error) {
	return tgc.ChannelByID(ctx, api, channelID)
}

func (g *telegramService) GetChannelFull(ctx context.Context, client TelegramClient, channelID int64) (*tg.Channel, error) {
	return tgc.GetChannelFull(ctx, client.API(), channelID)
}

func (g *telegramService) GetMediaContent(ctx context.Context, client TelegramClient, location tg.InputFileLocationClass) (*bytes.Buffer, error) {
	return tgc.GetMediaContent(ctx, client.API(), location)
}

func (g *telegramService) IsNoDefaultChannelError(err error) bool {
	return err == tgc.ErrNoDefaultChannel
}

func (g *telegramService) IsPasswordAuthNeeded(err error) bool {
	return errors.Is(err, tgauth.ErrPasswordAuthNeeded)
}

func (g *telegramService) IsSessionPasswordNeeded(err error) bool {
	return tgerr.Is(err, "SESSION_PASSWORD_NEEDED")
}

func (g *telegramService) IsAuthKeyUnregistered(err error) bool {
	return tgerr.Is(err, "AUTH_KEY_UNREGISTERED")
}

func (p *telegramUploadPool) Default(ctx context.Context) *tg.Client {
	return p.pool.Default(ctx)
}

func (p *telegramUploadPool) Close() {
	p.pool.Close()
}

func (c *telegramClient) API() *tg.Client {
	return c.client.API()
}

func (c *telegramClient) Run(ctx context.Context, f func(ctx context.Context) error) error {
	return c.client.Run(ctx, f)
}

func (c *telegramClient) RandInt64() (int64, error) {
	return c.client.RandInt64()
}

func (c *telegramClient) Self(ctx context.Context) (*TelegramUser, error) {
	user, err := c.client.Self(ctx)
	if err != nil {
		return nil, err
	}
	return toTelegramUser(user), nil
}

func (c *telegramClient) QRLogin(ctx context.Context, loggedIn qrlogin.LoggedIn, onToken func(ctx context.Context, tokenURL string) error) (*TelegramUser, error) {
	qr := c.client.QR()
	token, err := qr.Export(ctx)
	if err != nil {
		return nil, err
	}

	nextRefresh := func(token qrlogin.Token) time.Duration {
		until := time.Until(token.Expires().Add(-qrRenewBeforeExpiry)).Truncate(time.Second)
		if until < time.Second {
			return time.Second
		}
		return until
	}

	for {
		if err := onToken(ctx, token.URL()); err != nil {
			return nil, err
		}

		timer := time.NewTimer(nextRefresh(token))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
			token, err = qr.Export(ctx)
			if err != nil {
				return nil, err
			}
			continue
		case <-loggedIn:
			if !timer.Stop() {
				<-timer.C
			}
		}

		authorization, err := qr.Import(ctx)
		if err != nil {
			return nil, err
		}
		user, ok := authorization.User.AsNotEmpty()
		if !ok {
			return nil, fmt.Errorf("auth failed")
		}
		return toTelegramUser(user), nil
	}
}

func (c *telegramClient) SendCode(ctx context.Context, phoneNo string) (string, error) {
	response, err := c.client.Auth().SendCode(ctx, phoneNo, tgauth.SendCodeOptions{})
	if err != nil {
		return "", err
	}
	code, ok := response.(*tg.AuthSentCode)
	if !ok {
		return "", fmt.Errorf("unexpected send code response %T", response)
	}
	return code.PhoneCodeHash, nil
}

func (c *telegramClient) SignIn(ctx context.Context, phoneNo, phoneCode, phoneCodeHash string) (*TelegramUser, error) {
	authorization, err := c.client.Auth().SignIn(ctx, phoneNo, phoneCode, phoneCodeHash)
	if err != nil {
		return nil, err
	}
	user, ok := authorization.User.AsNotEmpty()
	if !ok {
		return nil, fmt.Errorf("auth failed")
	}
	return toTelegramUser(user), nil
}

func (c *telegramClient) Password(ctx context.Context, password string) (*TelegramUser, error) {
	authorization, err := c.client.Auth().Password(ctx, password)
	if err != nil {
		return nil, err
	}
	user, ok := authorization.User.AsNotEmpty()
	if !ok {
		return nil, fmt.Errorf("auth failed")
	}
	return toTelegramUser(user), nil
}

func unwrapClient(client TelegramClient) (*telegram.Client, error) {
	productionClient, ok := client.(*telegramClient)
	if !ok {
		return nil, fmt.Errorf("unsupported telegram client implementation")
	}
	return productionClient.client, nil
}

func toPartIDs(parts []api.Part) []int {
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		out = append(out, part.ID)
	}
	return out
}

func messageDocument(m *tg.Message) (*tg.Document, bool) {
	if m == nil {
		return nil, false
	}

	media, ok := m.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, false
	}

	document, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, false
	}

	return document, true
}

func toTelegramUser(user *tg.User) *TelegramUser {
	if user == nil {
		return nil
	}

	return &TelegramUser{
		ID:        user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Premium:   user.Premium,
	}
}
