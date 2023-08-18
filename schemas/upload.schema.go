package schemas

type UploadQuery struct {
	Filename   string `form:"fileName"`
	PartNo     int    `form:"partNo,omitempty"`
	TotalParts int    `form:"totalparts"`
}

type UploadPartOut struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PartId     int    `json:"partId"`
	PartNo     int    `json:"partNo"`
	TotalParts int    `json:"totalParts"`
	ChannelID  int64  `json:"channelId"`
	Size       int64  `json:"size"`
}

type UploadOut struct {
	Parts []UploadPartOut `json:"parts"`
}
