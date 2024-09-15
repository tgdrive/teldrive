package schemas

type ShareAccess struct {
	Password string `json:"password" binding:"required"`
}

type ShareFileQuery struct {
	Path  string `form:"path"`
	Sort  string `form:"sort"`
	Order string `form:"order"`
	Limit int    `form:"limit"`
	Page  int    `form:"page"`
}
