package schemas

type ShareAccess struct {
	Password string `json:"password" binding:"required"`
}

type ShareFileQuery struct {
	ParentID string `form:"parentId"`
	Sort     string `form:"sort"`
	Order    string `form:"order"`
	Limit    int    `form:"limit"`
	Page     int    `form:"page"`
}
