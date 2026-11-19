package telegramstore

import "context"

type attachmentDownloadKey struct{}

// WithAttachmentDownload selects fresh clients and all eligible download bots.
// Chunk concurrency remains governed by the normal download configuration.
func WithAttachmentDownload(ctx context.Context) context.Context {
	return context.WithValue(ctx, attachmentDownloadKey{}, true)
}

func IsAttachmentDownload(ctx context.Context) bool {
	value, _ := ctx.Value(attachmentDownloadKey{}).(bool)
	return value
}
