package schemas

type TgSession struct {
	Sesssion  string `json:"session"`
	UserID    int64  `json:"userId"`
	Bot       bool   `json:"bot"`
	UserName  string `json:"userName"`
	Name      string `json:"name"`
	IsPremium bool   `json:"isPremium"`
}

type Session struct {
	Name      string `json:"name"`
	UserName  string `json:"userName"`
	IsPremium bool   `json:"isPremium"`
	Hash      string `json:"hash"`
	Expires   string `json:"expires"`
}
