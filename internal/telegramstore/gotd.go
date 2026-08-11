package telegramstore

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"golang.org/x/sync/errgroup"
)

const (
	defaultUploadThreads = 4
	telegramUploadPart   = 512 * 1024
	telegramReadChunk    = 1024 * 1024
	telegramReadAlign    = 4 * 1024
	telegramReadBuffers  = 8
	telegramReadParallel = 4
	deleteBatchSize      = 100
)

// Runner owns Telegram authentication, client lifetime, bot selection, retry,
// rate-limit, and flood-wait middleware. The callback is invoked only while the
// underlying gotd client is running.
type Runner interface {
	Run(ctx context.Context, userID int64, operation Operation, fn func(context.Context, *tg.Client) error) error
}

// BotProvider resolves the upload bots that must be members of newly created
// storage channels. Returning an empty list is valid for user-only deployments.
type BotProvider interface {
	ChannelBots(ctx context.Context, userID int64, api *tg.Client) ([]tg.InputUserClass, error)
}

type GotdStorage struct {
	runner       Runner
	botProvider  BotProvider
	downloadPool *DownloadClientPool
}

type GotdStorageOption func(*GotdStorage)

func WithBotProvider(provider BotProvider) GotdStorageOption {
	return func(storage *GotdStorage) { storage.botProvider = provider }
}

func WithDownloadClientPool(pool *DownloadClientPool) GotdStorageOption {
	return func(storage *GotdStorage) { storage.downloadPool = pool }
}

func NewGotdStorage(runner Runner, options ...GotdStorageOption) *GotdStorage {
	storage := &GotdStorage{runner: runner}
	for _, option := range options {
		if option != nil {
			option(storage)
		}
	}
	return storage
}

func (s *GotdStorage) Upload(ctx context.Context, request UploadRequest) (StoredPart, error) {
	if s.runner == nil || request.UserID <= 0 || request.ChannelID == 0 || request.Reader == nil || request.Size < 0 || strings.TrimSpace(request.Name) == "" {
		return StoredPart{}, ErrInvalidRequest
	}
	threads := request.Threads
	if threads <= 0 {
		threads = defaultUploadThreads
	}

	var stored StoredPart
	err := s.runUpload(ctx, request.UserID, threads, func(runCtx context.Context, api *tg.Client) error {
		channel, err := inputChannel(runCtx, api, request.ChannelID)
		if err != nil {
			return err
		}
		uploaded, err := uploader.NewUploader(api).
			WithThreads(threads).
			WithPartSize(telegramUploadPart).
			Upload(runCtx, uploader.NewUpload(request.Name, request.Reader, request.Size))
		if err != nil {
			return fmt.Errorf("upload Telegram document bytes: %w", err)
		}

		document := message.UploadedDocument(uploaded).Filename(request.Name).ForceFile(true)
		response, err := message.NewSender(api).
			To(&tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash}).
			Media(runCtx, document)
		if err != nil {
			return fmt.Errorf("publish Telegram document message: %w", err)
		}
		messageID, documentSize, err := uploadedMessage(response)
		if err != nil {
			return err
		}
		if documentSize != request.Size {
			return fmt.Errorf("%w: got %d, want %d", ErrSizeMismatch, documentSize, request.Size)
		}
		stored = StoredPart{ChannelID: request.ChannelID, MessageID: messageID, Size: documentSize}
		return nil
	})
	if err != nil {
		return StoredPart{}, err
	}
	if stored.MessageID == 0 {
		return StoredPart{}, ErrMessageNotFound
	}
	return stored, nil
}

func (s *GotdStorage) runUpload(ctx context.Context, userID int64, threads int, fn func(context.Context, *tg.Client) error) error {
	if pooled, ok := s.runner.(PooledRunner); ok && threads > 1 {
		return pooled.RunPooled(ctx, userID, OperationUpload, threads, fn)
	}
	return s.runner.Run(ctx, userID, OperationUpload, fn)
}

func (s *GotdStorage) Metadata(ctx context.Context, request MetadataRequest) (StoredPart, error) {
	if s.runner == nil || request.UserID <= 0 || request.ChannelID == 0 || request.MessageID < 0 {
		return StoredPart{}, ErrInvalidRequest
	}
	var stored StoredPart
	err := s.runner.Run(ctx, request.UserID, OperationDownload, func(runCtx context.Context, api *tg.Client) error {
		var err error
		stored, err = metadataWithAPI(runCtx, api, request)
		return err
	})
	if err != nil {
		return StoredPart{}, err
	}
	return stored, nil
}

func metadataWithAPI(ctx context.Context, api *tg.Client, request MetadataRequest) (StoredPart, error) {
	_, size, err := documentLocation(ctx, api, request.ChannelID, request.MessageID)
	if err != nil {
		return StoredPart{}, err
	}
	return StoredPart{ChannelID: request.ChannelID, MessageID: request.MessageID, Size: size}, nil
}

func (s *GotdStorage) OpenRange(ctx context.Context, request RangeRequest) (io.ReadCloser, error) {
	if s.runner == nil || request.UserID <= 0 || request.ChannelID == 0 || request.MessageID <= 0 || request.Offset < 0 || request.Length < -1 {
		return nil, ErrInvalidRequest
	}
	streamCtx, cancel := context.WithCancel(ctx)
	reader := newTelegramRangeReader(streamCtx, cancel)
	go func() {
		err := s.runner.Run(streamCtx, request.UserID, OperationDownload, func(runCtx context.Context, api *tg.Client) error {
			return fillRangeWithAPI(runCtx, api, request, reader)
		})
		reader.finish(err)
	}()
	return reader, nil
}

func fillRangeWithAPI(ctx context.Context, api *tg.Client, request RangeRequest, reader *telegramRangeReader) error {
	location, documentSize, err := documentLocation(ctx, api, request.ChannelID, request.MessageID)
	if err != nil {
		return err
	}
	return fillRangeWithLocation(ctx, api, request, reader, location, documentSize)
}

func fillRangeWithLocation(ctx context.Context, api *tg.Client, request RangeRequest, reader *telegramRangeReader, location *tg.InputDocumentFileLocation, documentSize int64) error {
	if request.Offset > documentSize {
		return io.EOF
	}
	remaining := request.Length
	if remaining < 0 || request.Offset+remaining > documentSize {
		remaining = documentSize - request.Offset
	}
	return reader.fill(ctx, api, location, request.Offset, remaining)
}

type gotdDownloadSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	done     chan struct{}
	api      *tg.Client
	err      error
	clientFn func() (*tg.Client, error)
	closeFn  func() error
	close    sync.Once
	mu       sync.Mutex

	locationMu sync.Mutex
	locations  map[documentLocationKey]cachedDocumentLocation
}

type documentLocationKey struct {
	channelID int64
	messageID int64
}

type cachedDocumentLocation struct {
	location *tg.InputDocumentFileLocation
	size     int64
}

func (s *GotdStorage) OpenDownloadSession(ctx context.Context, userID int64) (DownloadSession, error) {
	if s == nil || s.runner == nil || userID <= 0 {
		return nil, ErrInvalidRequest
	}
	if s.downloadPool != nil {
		return s.downloadPool.OpenDownloadSession(ctx, userID)
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &gotdDownloadSession{
		ctx: sessionCtx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}),
	}
	go func() {
		err := s.runner.Run(sessionCtx, userID, OperationDownload, func(runCtx context.Context, api *tg.Client) error {
			session.mu.Lock()
			session.api = api
			session.mu.Unlock()
			close(session.ready)
			<-runCtx.Done()
			return runCtx.Err()
		})
		session.mu.Lock()
		session.err = err
		session.mu.Unlock()
		close(session.done)
	}()
	select {
	case <-session.ready:
		return session, nil
	case <-session.done:
		session.mu.Lock()
		err := session.err
		session.mu.Unlock()
		cancel()
		if err == nil {
			err = ErrClientUnavailable
		}
		return nil, err
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

func (s *gotdDownloadSession) Metadata(ctx context.Context, request MetadataRequest) (StoredPart, error) {
	api, err := s.client()
	if err != nil {
		return StoredPart{}, err
	}
	_, size, err := s.documentLocation(ctx, api, request.ChannelID, request.MessageID)
	if err != nil {
		return StoredPart{}, err
	}
	return StoredPart{ChannelID: request.ChannelID, MessageID: request.MessageID, Size: size}, nil
}

func (s *gotdDownloadSession) OpenRange(ctx context.Context, request RangeRequest) (io.ReadCloser, error) {
	api, err := s.client()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	reader := newTelegramRangeReader(streamCtx, cancel)
	go func() {
		location, size, locationErr := s.documentLocation(streamCtx, api, request.ChannelID, request.MessageID)
		if locationErr != nil {
			reader.finish(locationErr)
			return
		}
		reader.finish(fillRangeWithLocation(streamCtx, api, request, reader, location, size))
	}()
	return reader, nil
}

func (s *gotdDownloadSession) documentLocation(ctx context.Context, api *tg.Client, channelID, messageID int64) (*tg.InputDocumentFileLocation, int64, error) {
	key := documentLocationKey{channelID: channelID, messageID: messageID}
	s.locationMu.Lock()
	defer s.locationMu.Unlock()
	if cached, ok := s.locations[key]; ok {
		return cached.location, cached.size, nil
	}
	location, size, err := documentLocation(ctx, api, channelID, messageID)
	if err != nil {
		return nil, 0, err
	}
	if s.locations == nil {
		s.locations = make(map[documentLocationKey]cachedDocumentLocation)
	}
	s.locations[key] = cachedDocumentLocation{location: location, size: size}
	return location, size, nil
}

func (s *gotdDownloadSession) client() (*tg.Client, error) {
	if s.clientFn != nil {
		return s.clientFn()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api == nil {
		if s.err != nil {
			return nil, s.err
		}
		return nil, ErrClientUnavailable
	}
	return s.api, nil
}

func (s *gotdDownloadSession) Close() error {
	var err error
	s.close.Do(func() {
		if s.closeFn != nil {
			err = s.closeFn()
			return
		}
		s.cancel()
		<-s.done
	})
	return err
}

type telegramRangeReader struct {
	ctx        context.Context
	cancel     context.CancelFunc
	buffers    chan *telegramRangeBuffer
	done       chan struct{}
	cur        *telegramRangeBuffer
	readErr    error
	finishOnce sync.Once
	closeOnce  sync.Once
	mu         sync.Mutex
}

type telegramRangeBuffer struct {
	buf []byte
	off int
}

type telegramReadPlan struct {
	offset int64
	limit  int
	skip   int
	length int
}

func newTelegramRangeReader(ctx context.Context, cancel context.CancelFunc) *telegramRangeReader {
	return &telegramRangeReader{
		ctx:     ctx,
		cancel:  cancel,
		buffers: make(chan *telegramRangeBuffer, telegramReadBuffers),
		done:    make(chan struct{}),
	}
}

func (r *telegramRangeReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.cur == nil || r.cur.empty() {
		select {
		case buf, ok := <-r.buffers:
			if !ok {
				r.mu.Lock()
				err := r.readErr
				r.mu.Unlock()
				if err != nil && !errors.Is(err, context.Canceled) {
					return 0, err
				}
				return 0, io.EOF
			}
			r.cur = buf
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
	n := copy(p, r.cur.remaining())
	r.cur.off += n
	return n, nil
}

func (r *telegramRangeReader) Close() error {
	r.closeOnce.Do(func() { r.cancel() })
	select {
	case <-r.done:
	case <-r.ctx.Done():
	}
	return nil
}

func (r *telegramRangeReader) finish(err error) {
	r.finishOnce.Do(func() {
		r.mu.Lock()
		r.readErr = err
		r.mu.Unlock()
		close(r.buffers)
		close(r.done)
	})
}

func (r *telegramRangeReader) fill(ctx context.Context, api *tg.Client, location *tg.InputDocumentFileLocation, offset, remaining int64) error {
	for remaining > 0 {
		plans := planTelegramReads(offset, remaining, telegramReadParallel)
		buffers := make([][]byte, len(plans))
		g, groupCtx := errgroup.WithContext(ctx)
		g.SetLimit(telegramReadParallel)
		for i, plan := range plans {
			i := i
			plan := plan
			g.Go(func() error {
				response, err := api.UploadGetFile(groupCtx, &tg.UploadGetFileRequest{
					Location: location,
					Offset:   plan.offset,
					Limit:    plan.limit,
					Precise:  true,
				})
				if err != nil {
					return fmt.Errorf("download Telegram document chunk at %d: %w", plan.offset, err)
				}
				file, ok := response.(*tg.UploadFile)
				if !ok {
					return fmt.Errorf("unexpected Telegram download response %T", response)
				}
				end := plan.skip + plan.length
				if len(file.Bytes) < end {
					return io.ErrUnexpectedEOF
				}
				buffers[i] = file.Bytes[plan.skip:end]
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		for _, payload := range buffers {
			select {
			case r.buffers <- &telegramRangeBuffer{buf: payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		for _, plan := range plans {
			offset += int64(plan.length)
			remaining -= int64(plan.length)
		}
	}
	return nil
}

func planTelegramReads(offset, remaining int64, count int) []telegramReadPlan {
	plans := make([]telegramReadPlan, 0, min(count, telegramReadParallel))
	for remaining > 0 && len(plans) < count {
		requestOffset := offset / telegramReadAlign * telegramReadAlign
		skip := int(offset - requestOffset)
		boundaryRemaining := telegramReadChunk - int(requestOffset%telegramReadChunk)
		maxLimit := telegramReadAlign
		for maxLimit*2 <= boundaryRemaining {
			maxLimit *= 2
		}
		length := int(min(remaining, int64(maxLimit-skip)))
		limit := telegramReadAlign
		for limit < skip+length {
			limit *= 2
		}
		plans = append(plans, telegramReadPlan{
			offset: requestOffset, limit: limit, skip: skip, length: length,
		})
		offset += int64(length)
		remaining -= int64(length)
	}
	return plans
}

func (b *telegramRangeBuffer) empty() bool {
	return b == nil || len(b.buf)-b.off <= 0
}

func (b *telegramRangeBuffer) remaining() []byte {
	return b.buf[b.off:]
}

func (s *GotdStorage) CopyPart(ctx context.Context, userID, sourceChannelID, sourceMessageID, destinationChannelID int64) (StoredPart, error) {
	if s.runner == nil || userID <= 0 || sourceChannelID == 0 || sourceMessageID <= 0 || destinationChannelID == 0 {
		return StoredPart{}, ErrInvalidRequest
	}
	var copied StoredPart
	err := s.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		location, size, err := documentLocation(runCtx, api, sourceChannelID, sourceMessageID)
		if err != nil {
			return err
		}
		destination, err := inputChannel(runCtx, api, destinationChannelID)
		if err != nil {
			return err
		}
		var randomBytes [8]byte
		if _, err := cryptorand.Read(randomBytes[:]); err != nil {
			return fmt.Errorf("generate Telegram copy random id: %w", err)
		}
		response, err := api.MessagesSendMedia(runCtx, &tg.MessagesSendMediaRequest{
			Silent:   true,
			Peer:     &tg.InputPeerChannel{ChannelID: destination.ChannelID, AccessHash: destination.AccessHash},
			Media:    &tg.InputMediaDocument{ID: &tg.InputDocument{ID: location.ID, AccessHash: location.AccessHash, FileReference: location.FileReference}},
			RandomID: int64(binary.BigEndian.Uint64(randomBytes[:])),
		})
		if err != nil {
			return fmt.Errorf("copy Telegram document: %w", err)
		}
		messageID, copiedSize, err := uploadedMessage(response)
		if err != nil {
			return err
		}
		if copiedSize != size {
			return fmt.Errorf("%w: copied %d, source %d", ErrSizeMismatch, copiedSize, size)
		}
		copied = StoredPart{ChannelID: destinationChannelID, MessageID: messageID, Size: copiedSize}
		return nil
	})
	if err != nil {
		return StoredPart{}, err
	}
	return copied, nil
}

func (s *GotdStorage) DeleteMessages(ctx context.Context, userID, channelID int64, messageIDs []int64) error {
	if s.runner == nil || userID <= 0 || channelID == 0 {
		return ErrInvalidRequest
	}
	if len(messageIDs) == 0 {
		return nil
	}
	return s.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		channel, err := inputChannel(runCtx, api, channelID)
		if err != nil {
			return err
		}
		for start := 0; start < len(messageIDs); start += deleteBatchSize {
			end := min(start+deleteBatchSize, len(messageIDs))
			ids := make([]int, 0, end-start)
			for _, id := range messageIDs[start:end] {
				if id <= 0 {
					return ErrInvalidRequest
				}
				ids = append(ids, int(id))
			}
			if _, err := api.ChannelsDeleteMessages(runCtx, &tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: ids}); err != nil {
				return fmt.Errorf("delete Telegram messages: %w", err)
			}
		}
		return nil
	})
}

func (s *GotdStorage) ListDocumentMessages(ctx context.Context, request ListDocumentMessagesRequest) (DocumentMessagePage, error) {
	if s.runner == nil || request.UserID <= 0 || request.ChannelID == 0 || request.BeforeID < 0 || request.Limit <= 0 || request.Limit > 100 {
		return DocumentMessagePage{}, ErrInvalidRequest
	}
	var page DocumentMessagePage
	err := s.runner.Run(ctx, request.UserID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		channel, err := inputChannel(runCtx, api, request.ChannelID)
		if err != nil {
			return err
		}
		result, err := api.MessagesGetHistory(runCtx, &tg.MessagesGetHistoryRequest{
			Peer:     &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
			OffsetID: int(request.BeforeID), Limit: request.Limit,
		})
		if err != nil {
			return fmt.Errorf("list Telegram channel history: %w", err)
		}
		modified, ok := result.AsModified()
		if !ok {
			return errors.New("list Telegram channel history: unexpected unmodified response")
		}
		messages := modified.GetMessages()
		page.Exhausted = len(messages) < request.Limit
		for _, item := range messages {
			message, ok := item.(*tg.Message)
			if !ok {
				continue
			}
			if page.BeforeID == 0 || int64(message.ID) < page.BeforeID {
				page.BeforeID = int64(message.ID)
			}
			if _, ok := message.Media.(*tg.MessageMediaDocument); ok {
				page.Messages = append(page.Messages, DocumentMessage{ID: int64(message.ID), CreatedAt: time.Unix(int64(message.Date), 0).UTC()})
			}
		}
		if len(messages) > 0 && page.BeforeID == 0 {
			return errors.New("list Telegram channel history: page has no usable message ID")
		}
		return nil
	})
	if err != nil {
		return DocumentMessagePage{}, err
	}
	return page, nil
}

func (s *GotdStorage) CreateChannel(ctx context.Context, userID int64, name string) (Channel, error) {
	if s.runner == nil || userID <= 0 || strings.TrimSpace(name) == "" {
		return Channel{}, ErrInvalidRequest
	}
	var created Channel
	err := s.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		response, err := api.ChannelsCreateChannel(runCtx, &tg.ChannelsCreateChannelRequest{
			Title:     strings.TrimSpace(name),
			Broadcast: true,
		})
		if err != nil {
			return fmt.Errorf("create Telegram channel: %w", err)
		}
		updates, ok := response.(*tg.Updates)
		if !ok {
			return fmt.Errorf("unexpected Telegram channel response %T", response)
		}
		for _, chat := range updates.Chats {
			channel, ok := chat.(*tg.Channel)
			if !ok {
				continue
			}
			if s.botProvider != nil {
				bots, botErr := s.botProvider.ChannelBots(runCtx, userID, api)
				if botErr != nil {
					_, _ = api.ChannelsDeleteChannel(runCtx, channel.AsInput())
					return fmt.Errorf("resolve Telegram upload bots: %w", botErr)
				}
				for _, bot := range bots {
					if adminErr := setBotAdmin(runCtx, api, channel.AsInput(), bot); adminErr != nil {
						_, _ = api.ChannelsDeleteChannel(runCtx, channel.AsInput())
						return fmt.Errorf("add Telegram upload bot as channel admin: %w", adminErr)
					}
				}
			}
			created = Channel{ID: channel.ID, Name: channel.Title}
			return nil
		}
		return ErrInvalidChannel
	})
	if err != nil {
		return Channel{}, err
	}
	return created, nil
}

func (s *GotdStorage) DeleteChannel(ctx context.Context, userID, channelID int64) error {
	if s.runner == nil || userID <= 0 || channelID == 0 {
		return ErrInvalidRequest
	}
	return s.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		channel, err := fullChannel(runCtx, api, channelID)
		if err != nil {
			return err
		}
		if _, err := api.ChannelsDeleteChannel(runCtx, channel.AsInput()); err != nil {
			return fmt.Errorf("delete Telegram channel: %w", err)
		}
		return nil
	})
}

func (s *GotdStorage) InviteBot(ctx context.Context, userID, channelID int64, username string) error {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if s.runner == nil || userID <= 0 || channelID == 0 || username == "" {
		return ErrInvalidRequest
	}
	return s.runner.Run(ctx, userID, OperationManage, func(runCtx context.Context, api *tg.Client) error {
		slog.InfoContext(runCtx, "Resolving Telegram channel for bot provisioning",
			"user_id", userID,
			"channel_id", channelID,
			"bot_username", username,
		)
		channel, err := fullChannel(runCtx, api, channelID)
		if err != nil {
			return err
		}
		resolved, err := api.ContactsResolveUsername(runCtx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			return fmt.Errorf("resolve Telegram bot %s: %w", username, err)
		}
		var bot tg.InputUserClass
		var botID int64
		for _, item := range resolved.Users {
			user, ok := item.(*tg.User)
			if ok && user.Bot && strings.EqualFold(user.Username, username) {
				bot = user.AsInput()
				botID = user.ID
				break
			}
		}
		if bot == nil {
			return fmt.Errorf("resolve Telegram bot %s: %w", username, ErrInvalidRequest)
		}
		slog.InfoContext(runCtx, "Promoting Telegram bot to channel admin",
			"user_id", userID,
			"channel_id", channelID,
			"bot_id", botID,
			"bot_username", username,
		)
		if err := setBotAdmin(runCtx, api, channel.AsInput(), bot); err != nil {
			return fmt.Errorf("add Telegram bot %s as admin in channel %d: %w", username, channelID, err)
		}
		slog.InfoContext(runCtx, "Telegram bot promoted to channel admin",
			"user_id", userID,
			"channel_id", channelID,
			"bot_id", botID,
			"bot_username", username,
		)
		return nil
	})
}

func setBotAdmin(ctx context.Context, api *tg.Client, channel tg.InputChannelClass, bot tg.InputUserClass) error {
	if api == nil || channel == nil || bot == nil {
		return ErrInvalidRequest
	}
	_, err := api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
		Channel: channel,
		UserID:  bot,
		AdminRights: tg.ChatAdminRights{
			ChangeInfo:     true,
			PostMessages:   true,
			EditMessages:   true,
			DeleteMessages: true,
			BanUsers:       true,
			InviteUsers:    true,
			PinMessages:    true,
			ManageCall:     true,
			Other:          true,
			ManageTopics:   true,
		},
		Rank: "bot",
	})
	return err
}

func inputChannel(ctx context.Context, api *tg.Client, channelID int64) (*tg.InputChannel, error) {
	channel, err := fullChannel(ctx, api, channelID)
	if err != nil {
		return nil, err
	}
	return channel.AsInput(), nil
}

func fullChannel(ctx context.Context, api *tg.Client, channelID int64) (*tg.Channel, error) {
	response, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{&tg.InputChannel{ChannelID: channelID}})
	if err != nil {
		return nil, fmt.Errorf("resolve Telegram channel: %w", err)
	}
	chats := response.GetChats()
	if len(chats) == 0 {
		return nil, ErrInvalidChannel
	}
	channel, ok := chats[0].(*tg.Channel)
	if !ok {
		return nil, fmt.Errorf("unexpected Telegram chat %T", chats[0])
	}
	return channel, nil
}

func uploadedMessage(response tg.UpdatesClass) (int64, int64, error) {
	updates, ok := response.(*tg.Updates)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected Telegram upload response %T", response)
	}
	for _, update := range updates.Updates {
		channelMessage, ok := update.(*tg.UpdateNewChannelMessage)
		if !ok {
			continue
		}
		msg, ok := channelMessage.Message.(*tg.Message)
		if !ok {
			continue
		}
		document, ok := messageDocument(msg)
		if !ok {
			return 0, 0, ErrDocumentNotFound
		}
		return int64(msg.ID), document.Size, nil
	}
	return 0, 0, ErrMessageNotFound
}

func documentLocation(ctx context.Context, api *tg.Client, channelID, messageID int64) (*tg.InputDocumentFileLocation, int64, error) {
	channel, err := inputChannel(ctx, api, channelID)
	if err != nil {
		return nil, 0, err
	}
	response, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: int(messageID)}},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("get Telegram document message: %w", err)
	}
	modified, ok := response.AsModified()
	if !ok || len(modified.GetMessages()) == 0 {
		return nil, 0, ErrMessageNotFound
	}
	msg, ok := modified.GetMessages()[0].(*tg.Message)
	if !ok {
		return nil, 0, ErrMessageNotFound
	}
	document, ok := messageDocument(msg)
	if !ok {
		return nil, 0, ErrDocumentNotFound
	}
	return document.AsInputDocumentFileLocation(), document.Size, nil
}

func messageDocument(msg *tg.Message) (*tg.Document, bool) {
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, false
	}
	document, ok := media.Document.(*tg.Document)
	return document, ok
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error {
	r.cancel()
	return r.ReadCloser.Close()
}
