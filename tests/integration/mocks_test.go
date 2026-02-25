package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/pkg/dto"
	"github.com/tgdrive/teldrive/pkg/services"
	"github.com/tgdrive/teldrive/pkg/types"
)

var errUnexpectedTelegramCall = errors.New("unexpected telegram call")

type mockTelegramClient struct{}

type authFlowMockClient struct {
	qrLoginFn  func(ctx context.Context, loggedIn qrlogin.LoggedIn, onToken func(context.Context, string) error) (*services.TelegramUser, error)
	sendCodeFn func(ctx context.Context, phone string) (string, error)
	signInFn   func(ctx context.Context, phone, code, hash string) (*services.TelegramUser, error)
	passwordFn func(ctx context.Context, password string) (*services.TelegramUser, error)
}

func (m *mockTelegramClient) API() *tg.Client { return nil }

func (m *mockTelegramClient) Run(ctx context.Context, f func(context.Context) error) error {
	if f == nil {
		return nil
	}
	return f(ctx)
}

func (m *mockTelegramClient) RandInt64() (int64, error) { return 0, nil }

func (m *mockTelegramClient) Self(context.Context) (*services.TelegramUser, error) {
	return &services.TelegramUser{}, nil
}

func (m *mockTelegramClient) QRLogin(context.Context, qrlogin.LoggedIn, func(context.Context, string) error) (*services.TelegramUser, error) {
	return &services.TelegramUser{}, nil
}

func (m *mockTelegramClient) SendCode(context.Context, string) (string, error) { return "", nil }

func (m *mockTelegramClient) SignIn(context.Context, string, string, string) (*services.TelegramUser, error) {
	return &services.TelegramUser{}, nil
}

func (m *mockTelegramClient) Password(context.Context, string) (*services.TelegramUser, error) {
	return &services.TelegramUser{}, nil
}

func (m *authFlowMockClient) API() *tg.Client { return nil }

func (m *authFlowMockClient) Run(ctx context.Context, f func(context.Context) error) error {
	if f == nil {
		return nil
	}
	return f(ctx)
}

func (m *authFlowMockClient) RandInt64() (int64, error) { return 0, nil }

func (m *authFlowMockClient) Self(context.Context) (*services.TelegramUser, error) {
	return &services.TelegramUser{}, nil
}

func (m *authFlowMockClient) QRLogin(ctx context.Context, loggedIn qrlogin.LoggedIn, onToken func(context.Context, string) error) (*services.TelegramUser, error) {
	if m.qrLoginFn != nil {
		return m.qrLoginFn(ctx, loggedIn, onToken)
	}
	return &services.TelegramUser{}, nil
}

func (m *authFlowMockClient) SendCode(ctx context.Context, phone string) (string, error) {
	if m.sendCodeFn != nil {
		return m.sendCodeFn(ctx, phone)
	}
	return "", nil
}

func (m *authFlowMockClient) SignIn(ctx context.Context, phone, code, hash string) (*services.TelegramUser, error) {
	if m.signInFn != nil {
		return m.signInFn(ctx, phone, code, hash)
	}
	return &services.TelegramUser{}, nil
}

func (m *authFlowMockClient) Password(ctx context.Context, password string) (*services.TelegramUser, error) {
	if m.passwordFn != nil {
		return m.passwordFn(ctx, password)
	}
	return &services.TelegramUser{}, nil
}

type mockUploadPool struct{}

func (m *mockUploadPool) Default(context.Context) *tg.Client { return nil }
func (m *mockUploadPool) Close()                             {}

type mockTelegramService struct {
	authClientFn     func(ctx context.Context, session string, retries int) (services.TelegramClient, error)
	botClientFn      func(ctx context.Context, token string, retries int) (services.TelegramClient, error)
	runWithAuthFn    func(ctx context.Context, client services.TelegramClient, token string, f func(context.Context) error) error
	newUploadPoolFn  func(ctx context.Context, client services.TelegramClient, poolSize int64, maxRetries int) (services.UploadPool, error)
	deleteChannelFn  func(ctx context.Context, client services.TelegramClient, channelID int64) (storage.PeerKey, error)
	syncDialogsFn    func(ctx context.Context, client services.TelegramClient, peerStorage storage.PeerStorage) error
	listAuthsFn      func(ctx context.Context, client services.TelegramClient) ([]services.TelegramAuthorization, error)
	logoutFn         func(ctx context.Context, client services.TelegramClient) error
	profilePhotoFn   func(ctx context.Context, client services.TelegramClient) ([]byte, int64, bool, error)
	getMessagesFn    func(ctx context.Context, client services.TelegramClient, ids []int, channelID int64) ([]tg.MessageClass, error)
	getPartsFn       func(ctx context.Context, client services.TelegramClient, channelID int64, parts []api.Part, encrypted bool) ([]types.Part, error)
	copyFilePartsFn  func(ctx context.Context, client services.TelegramClient, fromChannelID int64, toChannelID int64, parts []api.Part) ([]api.Part, error)
	selectBotTokenFn func(ctx context.Context, operation string, userID int64, tokens []string) (string, int, error)
	uploadPartFn     func(ctx context.Context, apiClient *tg.Client, channelID int64, partName string, fileStream io.Reader, fileSize int64, threads int) (int, int64, error)
	noAuthClientFn   func(ctx context.Context, dispatcher tg.UpdateDispatcher, storage session.Storage) (services.TelegramClient, error)
	passwordAuthFn   func(err error) bool
	sessionPwAuthFn  func(err error) bool
	noDefaultErrFn   func(err error) bool
}

func newMockTelegramService() *mockTelegramService { return &mockTelegramService{} }

func (m *mockTelegramService) AuthClient(ctx context.Context, session string, retries int) (services.TelegramClient, error) {
	if m.authClientFn != nil {
		return m.authClientFn(ctx, session, retries)
	}
	return &mockTelegramClient{}, nil
}

func (m *mockTelegramService) BotClient(ctx context.Context, token string, retries int) (services.TelegramClient, error) {
	if m.botClientFn != nil {
		return m.botClientFn(ctx, token, retries)
	}
	return &mockTelegramClient{}, nil
}

func (m *mockTelegramService) SelectBotToken(ctx context.Context, operation string, userID int64, tokens []string) (string, int, error) {
	if m.selectBotTokenFn != nil {
		return m.selectBotTokenFn(ctx, operation, userID, tokens)
	}
	if len(tokens) == 0 {
		return "", 0, errors.New("no bot tokens")
	}
	return tokens[0], 0, nil
}

func (m *mockTelegramService) NewQRLogin() (tg.UpdateDispatcher, qrlogin.LoggedIn) {
	d := tg.NewUpdateDispatcher()
	return d, qrlogin.OnLoginToken(d)
}

func (m *mockTelegramService) NoAuthClient(ctx context.Context, dispatcher tg.UpdateDispatcher, storage session.Storage) (services.TelegramClient, error) {
	if m.noAuthClientFn != nil {
		return m.noAuthClientFn(ctx, dispatcher, storage)
	}
	return &mockTelegramClient{}, nil
}

func (m *mockTelegramService) RunWithAuth(ctx context.Context, c services.TelegramClient, token string, f func(context.Context) error) error {
	if m.runWithAuthFn != nil {
		return m.runWithAuthFn(ctx, c, token, f)
	}
	if f == nil {
		return nil
	}
	return f(ctx)
}

func (m *mockTelegramService) LogOut(ctx context.Context, c services.TelegramClient) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, c)
	}
	return nil
}

func (m *mockTelegramService) ListAuthorizations(ctx context.Context, c services.TelegramClient) ([]services.TelegramAuthorization, error) {
	if m.listAuthsFn != nil {
		return m.listAuthsFn(ctx, c)
	}
	return []services.TelegramAuthorization{}, nil
}

func (m *mockTelegramService) DeleteChannel(ctx context.Context, c services.TelegramClient, channelID int64) (storage.PeerKey, error) {
	if m.deleteChannelFn != nil {
		return m.deleteChannelFn(ctx, c, channelID)
	}
	return storage.PeerKey{}, nil
}

func (m *mockTelegramService) SyncDialogs(ctx context.Context, c services.TelegramClient, peerStorage storage.PeerStorage) error {
	if m.syncDialogsFn != nil {
		return m.syncDialogsFn(ctx, c, peerStorage)
	}
	return nil
}

func (m *mockTelegramService) GetProfilePhoto(ctx context.Context, c services.TelegramClient) ([]byte, int64, bool, error) {
	if m.profilePhotoFn != nil {
		return m.profilePhotoFn(ctx, c)
	}
	return nil, 0, false, nil
}

func (m *mockTelegramService) NewUploadPool(ctx context.Context, c services.TelegramClient, poolSize int64, maxRetries int) (services.UploadPool, error) {
	if m.newUploadPoolFn != nil {
		return m.newUploadPoolFn(ctx, c, poolSize, maxRetries)
	}
	return &mockUploadPool{}, nil
}

func (m *mockTelegramService) GetMessages(ctx context.Context, client services.TelegramClient, ids []int, channelID int64) ([]tg.MessageClass, error) {
	if m.getMessagesFn != nil {
		return m.getMessagesFn(ctx, client, ids, channelID)
	}
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) GetParts(ctx context.Context, client services.TelegramClient, channelID int64, parts []api.Part, encrypted bool) ([]types.Part, error) {
	if m.getPartsFn != nil {
		return m.getPartsFn(ctx, client, channelID, parts, encrypted)
	}
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) CopyFileParts(ctx context.Context, client services.TelegramClient, fromChannelID int64, toChannelID int64, parts []api.Part) ([]api.Part, error) {
	if m.copyFilePartsFn != nil {
		return m.copyFilePartsFn(ctx, client, fromChannelID, toChannelID, parts)
	}
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) UploadPart(ctx context.Context, apiClient *tg.Client, channelID int64, partName string, fileStream io.Reader, fileSize int64, threads int) (int, int64, error) {
	if m.uploadPartFn != nil {
		return m.uploadPartFn(ctx, apiClient, channelID, partName, fileStream, fileSize, threads)
	}
	return 0, 0, errUnexpectedTelegramCall
}

func (m *mockTelegramService) ChannelByID(context.Context, services.TelegramClient, int64) (*tg.InputChannel, error) {
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) ChannelByIDRaw(context.Context, *tg.Client, int64) (*tg.InputChannel, error) {
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) GetChannelFull(context.Context, services.TelegramClient, int64) (*tg.Channel, error) {
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) GetMediaContent(context.Context, services.TelegramClient, tg.InputFileLocationClass) (*bytes.Buffer, error) {
	return nil, errUnexpectedTelegramCall
}

func (m *mockTelegramService) IsNoDefaultChannelError(err error) bool {
	if m.noDefaultErrFn != nil {
		return m.noDefaultErrFn(err)
	}
	return false
}
func (m *mockTelegramService) IsPasswordAuthNeeded(err error) bool {
	if m.passwordAuthFn != nil {
		return m.passwordAuthFn(err)
	}
	return false
}
func (m *mockTelegramService) IsSessionPasswordNeeded(err error) bool {
	if m.sessionPwAuthFn != nil {
		return m.sessionPwAuthFn(err)
	}
	return false
}
func (m *mockTelegramService) IsAuthKeyUnregistered(error) bool { return false }

type noopEventBroadcaster struct{}

func newNoopEventBroadcaster() *noopEventBroadcaster { return &noopEventBroadcaster{} }

func (n *noopEventBroadcaster) Subscribe(int64, []events.EventType) chan dto.Event {
	return make(chan dto.Event)
}

func (n *noopEventBroadcaster) Unsubscribe(int64, chan dto.Event)           {}
func (n *noopEventBroadcaster) Record(events.EventType, int64, *dto.Source) {}
func (n *noopEventBroadcaster) Shutdown()                                   {}

type noopJobClient struct{}

func newNoopJobClient() *noopJobClient { return &noopJobClient{} }

func (n *noopJobClient) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &rivertype.JobInsertResult{Job: &rivertype.JobRow{
		ID:          1,
		Kind:        args.Kind(),
		Queue:       river.QueueDefault,
		State:       rivertype.JobStateAvailable,
		Attempt:     0,
		MaxAttempts: 25,
		EncodedArgs: encodedArgs,
		CreatedAt:   now,
		ScheduledAt: now,
		Metadata:    []byte(`{}`),
	}}, nil
}

func (n *noopJobClient) JobList(context.Context, *river.JobListParams) (*river.JobListResult, error) {
	return &river.JobListResult{Jobs: nil, LastCursor: nil}, nil
}

func (n *noopJobClient) JobGet(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, rivertype.ErrNotFound
}

func (n *noopJobClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, rivertype.ErrNotFound
}
