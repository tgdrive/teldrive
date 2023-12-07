package schemas

type UploadQuery struct {
	Filename  string `form:"fileName" binding:"required"`
	PartNo    int    `form:"partNo" binding:"required"`
	ChannelID int64  `form:"channelId" binding:"required"`
	Encrypted bool   `form:"encrypted"`
}

type UploadPartOut struct {
	Name      string `json:"name"`
	PartId    int    `json:"partId"`
	PartNo    int    `json:"partNo"`
	ChannelID int64  `json:"channelId"`
	Size      int64  `json:"size"`
	Encrypted bool   `json:"encrypted"`
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
	Encrypted bool   `json:"encrypted"`
}
