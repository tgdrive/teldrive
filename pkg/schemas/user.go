package schemas

type Channel struct {
	ChannelID   int64  `json:"channelId"`
	ChannelName string `json:"channelName"`
}

type AccountStats struct {
	TotalSize  int64 `json:"totalSize"`
	TotalFiles int64 `json:"totalFiles"`
	Channel
}
