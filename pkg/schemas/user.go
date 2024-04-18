package schemas

type Channel struct {
	ChannelID   int64  `json:"channelId"`
	ChannelName string `json:"channelName"`
}

type AccountStats struct {
	ChannelID int64    `json:"channelId"`
	Bots      []string `json:"bots"`
}
