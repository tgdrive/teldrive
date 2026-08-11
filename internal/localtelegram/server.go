package localtelegram

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const (
	stateFileName    = "state.json"
	uploadsDirName   = "uploads"
	documentsDirName = "documents"
)

type Server struct {
	root         string
	statePath    string
	uploadsDir   string
	documentsDir string

	mu    sync.Mutex
	state persistedState
}

func Open(root string) (*Server, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("local Telegram root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local Telegram root: %w", err)
	}
	server := &Server{
		root:         root,
		statePath:    filepath.Join(root, stateFileName),
		uploadsDir:   filepath.Join(root, uploadsDirName),
		documentsDir: filepath.Join(root, documentsDirName),
	}
	for _, dir := range []string{server.root, server.uploadsDir, server.documentsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create local Telegram directory %q: %w", dir, err)
		}
	}
	state, err := loadState(server.statePath)
	if err != nil {
		return nil, err
	}
	server.state = state
	if err := server.recoverUploads(); err != nil {
		return nil, err
	}
	if err := saveState(server.statePath, server.state); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *Server) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Server) Client() *tg.Client {
	if s == nil {
		return nil
	}
	return tg.NewClient(s)
}

func (s *Server) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	if s == nil || input == nil || output == nil {
		return errors.New("local Telegram invoker is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	response, err := s.handle(ctx, input)
	if err != nil {
		return err
	}
	buffer := new(bin.Buffer)
	if err := response.Encode(buffer); err != nil {
		return fmt.Errorf("encode local Telegram response: %w", err)
	}
	if err := output.Decode(buffer); err != nil {
		return fmt.Errorf("decode local Telegram response: %w", err)
	}
	return nil
}

func (s *Server) handle(ctx context.Context, input bin.Encoder) (bin.Encoder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch request := input.(type) {
	case *tg.ChannelsGetChannelsRequest:
		return s.getChannels(request), nil
	case *tg.MessagesGetDialogsRequest:
		return s.getDialogs(request), nil
	case *tg.UsersGetUsersRequest:
		return s.getUsers(request), nil
	case *tg.UploadSaveFilePartRequest:
		return s.saveUploadPart(ctx, request.FileID, request.FilePart, request.Bytes)
	case *tg.UploadSaveBigFilePartRequest:
		return s.saveUploadPart(ctx, request.FileID, request.FilePart, request.Bytes)
	case *tg.MessagesSendMediaRequest:
		return s.sendMedia(ctx, request)
	case *tg.ChannelsGetMessagesRequest:
		return s.getMessages(request), nil
	case *tg.UploadGetFileRequest:
		return s.getFile(ctx, request)
	case *tg.ChannelsDeleteMessagesRequest:
		return s.deleteMessages(request)
	case *tg.ChannelsCreateChannelRequest:
		return s.createChannel(request)
	case *tg.ChannelsDeleteChannelRequest:
		return s.deleteChannel(request)
	default:
		return nil, fmt.Errorf("local Telegram RPC %T is not implemented", input)
	}
}

func (s *Server) getChannels(request *tg.ChannelsGetChannelsRequest) *tg.MessagesChats {
	chats := make([]tg.ChatClass, 0, len(request.ID))
	for _, input := range request.ID {
		channelID, ok := inputChannelID(input)
		if !ok {
			continue
		}
		if record, exists := s.state.Channels[channelKey(channelID)]; exists {
			chats = append(chats, telegramChannel(record))
		}
	}
	return &tg.MessagesChats{Chats: chats}
}

func (s *Server) getDialogs(request *tg.MessagesGetDialogsRequest) *tg.MessagesDialogs {
	channels := make([]channelRecord, 0, len(s.state.Channels))
	for _, channel := range s.state.Channels {
		channels = append(channels, channel)
	}
	slices.SortFunc(channels, func(a, b channelRecord) int { return cmp.Compare(a.ID, b.ID) })
	limit := request.Limit
	if limit <= 0 || limit > len(channels) {
		limit = len(channels)
	}
	dialogs := make([]tg.DialogClass, 0, limit)
	chats := make([]tg.ChatClass, 0, limit)
	for _, channel := range channels[:limit] {
		dialogs = append(dialogs, &tg.Dialog{
			Peer:           &tg.PeerChannel{ChannelID: channel.ID},
			NotifySettings: tg.PeerNotifySettings{},
		})
		chats = append(chats, telegramChannel(channel))
	}
	return &tg.MessagesDialogs{Dialogs: dialogs, Chats: chats}
}

func (s *Server) getUsers(*tg.UsersGetUsersRequest) *tg.UserClassVector {
	return &tg.UserClassVector{Elems: []tg.UserClass{&tg.User{
		Self: true, ID: 1, AccessHash: 1_000_001,
		FirstName: "Local", LastName: "Telegram", Username: "localtelegram",
	}}}
}

func (s *Server) saveUploadPart(ctx context.Context, fileID int64, part int, payload []byte) (bin.Encoder, error) {
	if fileID == 0 || part < 0 {
		return nil, errors.New("invalid local Telegram upload part")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.uploadsDir, fmt.Sprintf("%d", fileID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create local Telegram upload: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%08d.part", part))
	if err := writeAtomic(path, payload, 0o600); err != nil {
		return nil, fmt.Errorf("write local Telegram upload part: %w", err)
	}
	return &tg.BoolTrue{}, nil
}

func (s *Server) sendMedia(ctx context.Context, request *tg.MessagesSendMediaRequest) (bin.Encoder, error) {
	channelID, ok := inputPeerChannelID(request.Peer)
	if !ok {
		return nil, errors.New("local Telegram destination is not a channel")
	}
	channel, exists := s.state.Channels[channelKey(channelID)]
	if !exists {
		return nil, fmt.Errorf("local Telegram channel %d does not exist", channelID)
	}

	var document documentRecord
	switch media := request.Media.(type) {
	case *tg.InputMediaUploadedDocument:
		created, err := s.finalizeUpload(ctx, media)
		if err != nil {
			return nil, err
		}
		document = created
	case *tg.InputMediaDocument:
		input, ok := media.ID.(*tg.InputDocument)
		if !ok {
			return nil, fmt.Errorf("local Telegram document reference %T is not supported", media.ID)
		}
		stored, exists := s.state.Documents[documentKey(input.ID)]
		if !exists || stored.AccessHash != input.AccessHash {
			return nil, fmt.Errorf("local Telegram document %d does not exist", input.ID)
		}
		document = stored
	default:
		return nil, fmt.Errorf("local Telegram media %T is not supported", request.Media)
	}

	messageID := s.state.NextMessageID
	s.state.NextMessageID++
	message := messageRecord{
		ChannelID:  channelID,
		ID:         messageID,
		DocumentID: document.ID,
		CreatedAt:  int(time.Now().Unix()),
	}
	s.state.Messages[messageKey(channelID, messageID)] = message
	if err := saveState(s.statePath, s.state); err != nil {
		return nil, err
	}

	telegramMessage := s.telegramMessage(message)
	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateNewChannelMessage{Message: telegramMessage, Pts: messageID, PtsCount: 1}},
		Chats:   []tg.ChatClass{telegramChannel(channel)},
		Date:    message.CreatedAt,
		Seq:     messageID,
	}, nil
}

func (s *Server) finalizeUpload(ctx context.Context, media *tg.InputMediaUploadedDocument) (documentRecord, error) {
	fileID, parts, name, ok := inputFileDetails(media.File)
	if !ok || parts <= 0 {
		return documentRecord{}, fmt.Errorf("local Telegram input file %T is invalid", media.File)
	}
	uploadDir := filepath.Join(s.uploadsDir, fmt.Sprintf("%d", fileID))
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return documentRecord{}, fmt.Errorf("read local Telegram upload: %w", err)
	}
	if len(entries) != parts {
		return documentRecord{}, fmt.Errorf("local Telegram upload %d has %d parts, want %d", fileID, len(entries), parts)
	}

	documentID := s.state.NextDocumentID
	s.state.NextDocumentID++
	destination := filepath.Join(s.documentsDir, fmt.Sprintf("%d.bin", documentID))
	temp, err := os.CreateTemp(s.documentsDir, ".document-*.tmp")
	if err != nil {
		return documentRecord{}, fmt.Errorf("create local Telegram document temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()

	hash := sha256.New()
	writer := io.MultiWriter(temp, hash)
	var size int64
	for part := 0; part < parts; part++ {
		if err := ctx.Err(); err != nil {
			return documentRecord{}, err
		}
		partPath := filepath.Join(uploadDir, fmt.Sprintf("%08d.part", part))
		partFile, err := os.Open(partPath)
		if err != nil {
			return documentRecord{}, fmt.Errorf("open local Telegram upload part %d: %w", part, err)
		}
		written, copyErr := io.Copy(writer, partFile)
		closeErr := partFile.Close()
		if copyErr != nil {
			return documentRecord{}, fmt.Errorf("copy local Telegram upload part %d: %w", part, copyErr)
		}
		if closeErr != nil {
			return documentRecord{}, fmt.Errorf("close local Telegram upload part %d: %w", part, closeErr)
		}
		size += written
	}
	if err := temp.Sync(); err != nil {
		return documentRecord{}, fmt.Errorf("sync local Telegram document: %w", err)
	}
	if err := temp.Close(); err != nil {
		return documentRecord{}, fmt.Errorf("close local Telegram document: %w", err)
	}
	if err := os.Rename(tempName, destination); err != nil {
		return documentRecord{}, fmt.Errorf("publish local Telegram document: %w", err)
	}
	cleanup = false
	if err := os.RemoveAll(uploadDir); err != nil {
		return documentRecord{}, fmt.Errorf("remove local Telegram upload parts: %w", err)
	}

	mimeType := strings.TrimSpace(media.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if attributeName := filenameFromAttributes(media.Attributes); attributeName != "" {
		name = attributeName
	}
	reference := []byte(hex.EncodeToString(hash.Sum(nil)))
	record := documentRecord{
		ID:            documentID,
		AccessHash:    documentID + 1_000_000,
		FileReference: reference,
		MimeType:      mimeType,
		Size:          size,
		DCID:          1,
		FileName:      name,
		CreatedAt:     int(time.Now().Unix()),
	}
	s.state.Documents[documentKey(documentID)] = record
	return record, nil
}

func (s *Server) getMessages(request *tg.ChannelsGetMessagesRequest) *tg.MessagesMessages {
	channelID, ok := inputChannelID(request.Channel)
	if !ok {
		return &tg.MessagesMessages{}
	}
	messages := make([]tg.MessageClass, 0, len(request.ID))
	for _, input := range request.ID {
		messageID, ok := inputMessageID(input)
		if !ok {
			continue
		}
		if record, exists := s.state.Messages[messageKey(channelID, messageID)]; exists {
			messages = append(messages, s.telegramMessage(record))
		}
	}
	chats := make([]tg.ChatClass, 0, 1)
	if channel, exists := s.state.Channels[channelKey(channelID)]; exists {
		chats = append(chats, telegramChannel(channel))
	}
	return &tg.MessagesMessages{Messages: messages, Chats: chats}
}

func (s *Server) getFile(ctx context.Context, request *tg.UploadGetFileRequest) (bin.Encoder, error) {
	location, ok := request.Location.(*tg.InputDocumentFileLocation)
	if !ok {
		return nil, fmt.Errorf("local Telegram file location %T is not supported", request.Location)
	}
	document, exists := s.state.Documents[documentKey(location.ID)]
	if !exists || document.AccessHash != location.AccessHash {
		return nil, fmt.Errorf("local Telegram document %d does not exist", location.ID)
	}
	if request.Offset < 0 || request.Limit < 0 || request.Offset > document.Size {
		return nil, errors.New("invalid local Telegram file range")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(s.documentsDir, fmt.Sprintf("%d.bin", document.ID)))
	if err != nil {
		return nil, fmt.Errorf("open local Telegram document: %w", err)
	}
	defer file.Close()
	limit := int64(request.Limit)
	if request.Offset+limit > document.Size {
		limit = document.Size - request.Offset
	}
	payload := make([]byte, limit)
	if limit > 0 {
		if _, err := file.ReadAt(payload, request.Offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read local Telegram document: %w", err)
		}
	}
	return &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Mtime: document.CreatedAt, Bytes: payload}, nil
}

func (s *Server) deleteMessages(request *tg.ChannelsDeleteMessagesRequest) (bin.Encoder, error) {
	channelID, ok := inputChannelID(request.Channel)
	if !ok {
		return nil, errors.New("invalid local Telegram channel")
	}
	for _, messageID := range request.ID {
		delete(s.state.Messages, messageKey(channelID, messageID))
	}
	if err := s.garbageCollectDocuments(); err != nil {
		return nil, err
	}
	if err := saveState(s.statePath, s.state); err != nil {
		return nil, err
	}
	return &tg.MessagesAffectedMessages{Pts: s.state.NextMessageID, PtsCount: len(request.ID)}, nil
}

func (s *Server) createChannel(request *tg.ChannelsCreateChannelRequest) (bin.Encoder, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return nil, errors.New("local Telegram channel title is required")
	}
	channelID := s.state.NextChannelID
	s.state.NextChannelID++
	record := channelRecord{
		ID:         channelID,
		AccessHash: channelID + 1_000_000,
		Title:      title,
		CreatedAt:  int(time.Now().Unix()),
	}
	s.state.Channels[channelKey(channelID)] = record
	if err := saveState(s.statePath, s.state); err != nil {
		return nil, err
	}
	return &tg.Updates{Chats: []tg.ChatClass{telegramChannel(record)}, Date: record.CreatedAt, Seq: s.state.NextMessageID}, nil
}

func (s *Server) deleteChannel(request *tg.ChannelsDeleteChannelRequest) (bin.Encoder, error) {
	channelID, ok := inputChannelID(request.Channel)
	if !ok {
		return nil, errors.New("invalid local Telegram channel")
	}
	delete(s.state.Channels, channelKey(channelID))
	for key, message := range s.state.Messages {
		if message.ChannelID == channelID {
			delete(s.state.Messages, key)
		}
	}
	if err := s.garbageCollectDocuments(); err != nil {
		return nil, err
	}
	if err := saveState(s.statePath, s.state); err != nil {
		return nil, err
	}
	return &tg.Updates{Date: int(time.Now().Unix()), Seq: s.state.NextMessageID}, nil
}

func (s *Server) telegramMessage(record messageRecord) *tg.Message {
	document := s.state.Documents[documentKey(record.DocumentID)]
	return &tg.Message{
		ID:     record.ID,
		Out:    true,
		Post:   true,
		PeerID: &tg.PeerChannel{ChannelID: record.ChannelID},
		Date:   record.CreatedAt,
		Media:  &tg.MessageMediaDocument{Document: telegramDocument(document)},
	}
}

func (s *Server) garbageCollectDocuments() error {
	referenced := make(map[int64]struct{}, len(s.state.Messages))
	for _, message := range s.state.Messages {
		referenced[message.DocumentID] = struct{}{}
	}
	for key, document := range s.state.Documents {
		if _, ok := referenced[document.ID]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(s.documentsDir, fmt.Sprintf("%d.bin", document.ID))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove local Telegram document: %w", err)
		}
		delete(s.state.Documents, key)
	}
	return nil
}

func (s *Server) recoverUploads() error {
	entries, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return fmt.Errorf("read local Telegram uploads: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.uploadsDir, entry.Name())); err != nil {
			return fmt.Errorf("remove incomplete local Telegram upload %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func telegramChannel(record channelRecord) *tg.Channel {
	return &tg.Channel{
		Creator:    true,
		Broadcast:  true,
		ID:         record.ID,
		AccessHash: record.AccessHash,
		Title:      record.Title,
		Photo:      &tg.ChatPhotoEmpty{},
		Date:       record.CreatedAt,
	}
}

func telegramDocument(record documentRecord) *tg.Document {
	attributes := make([]tg.DocumentAttributeClass, 0, 1)
	if record.FileName != "" {
		attributes = append(attributes, &tg.DocumentAttributeFilename{FileName: record.FileName})
	}
	return &tg.Document{
		ID:            record.ID,
		AccessHash:    record.AccessHash,
		FileReference: append([]byte(nil), record.FileReference...),
		Date:          record.CreatedAt,
		MimeType:      record.MimeType,
		Size:          record.Size,
		DCID:          record.DCID,
		Attributes:    attributes,
	}
}

func writeAtomic(path string, payload []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".part-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func inputChannelID(input tg.InputChannelClass) (int64, bool) {
	channel, ok := input.(*tg.InputChannel)
	if !ok || channel.ChannelID == 0 {
		return 0, false
	}
	return channel.ChannelID, true
}

func inputPeerChannelID(input tg.InputPeerClass) (int64, bool) {
	channel, ok := input.(*tg.InputPeerChannel)
	if !ok || channel.ChannelID == 0 {
		return 0, false
	}
	return channel.ChannelID, true
}

func inputMessageID(input tg.InputMessageClass) (int, bool) {
	message, ok := input.(*tg.InputMessageID)
	if !ok || message.ID <= 0 {
		return 0, false
	}
	return message.ID, true
}

func inputFileDetails(input tg.InputFileClass) (id int64, parts int, name string, ok bool) {
	switch file := input.(type) {
	case *tg.InputFile:
		return file.ID, file.Parts, file.Name, file.ID != 0 && file.Parts > 0
	case *tg.InputFileBig:
		return file.ID, file.Parts, file.Name, file.ID != 0 && file.Parts > 0
	default:
		return 0, 0, "", false
	}
}

func filenameFromAttributes(attributes []tg.DocumentAttributeClass) string {
	for _, attribute := range attributes {
		if filename, ok := attribute.(*tg.DocumentAttributeFilename); ok {
			return strings.TrimSpace(filename.FileName)
		}
	}
	return ""
}

type Runner struct {
	client *tg.Client
}

func NewRunner(server *Server) (Runner, error) {
	if server == nil {
		return Runner{}, errors.New("local Telegram server is required")
	}
	return Runner{client: server.Client()}, nil
}

func (r Runner) Run(ctx context.Context, userID int64, _ telegramstore.Operation, fn func(context.Context, *tg.Client) error) error {
	if r.client == nil || userID <= 0 || fn == nil {
		return telegramstore.ErrInvalidRequest
	}
	return fn(ctx, r.client)
}

var _ tg.Invoker = (*Server)(nil)
var _ telegramstore.Runner = Runner{}
