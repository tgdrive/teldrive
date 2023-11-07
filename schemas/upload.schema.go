package schemas

type UploadQuery struct {
	Filename   string `form:"fileName"`
	PartNo     int    `form:"partNo,omitempty"`
	TotalParts int    `form:"totalparts"`
	ChannelID  int64  `form:"channelId"`
}

type UploadPartOut struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PartId    int    `json:"partId"`
	PartNo    int    `json:"partNo"`
	ChannelID int64  `json:"channelId"`
	Size      int64  `json:"size"`
}

type UploadOut struct {
	Parts []UploadPartOut `json:"parts"`
}

type UploadPart struct {
	Name      string `json:"name"`
	UploadId  string `json:"uploadId"`
	PartId    int    `json:"partId"`
	PartNo    int    `json:"partNo"`
	ChannelID int64  `json:"channelId"`
	Size      int64  `json:"size"`
}
