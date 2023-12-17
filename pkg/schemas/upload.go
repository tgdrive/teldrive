package schemas

type UploadQuery struct {
	PartName  string `form:"partName" binding:"required"`
	FileName  string `form:"fileName" binding:"required"`
	PartNo    int    `form:"partNo" binding:"required"`
	ChannelID int64  `form:"channelId"`
	Encrypted bool   `form:"encrypted"`
}

type UploadPartOut struct {
	Name      string `json:"name"`
	PartId    int    `json:"partId"`
	PartNo    int    `json:"partNo"`
	ChannelID int64  `json:"channelId"`
	Size      int64  `json:"size"`
	Encrypted bool   `json:"encrypted"`
	Salt      string `json:"salt"`
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
