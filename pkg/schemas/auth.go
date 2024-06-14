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
	UserId    int64  `json:"userId"`
	IsPremium bool   `json:"isPremium"`
	Hash      string `json:"hash"`
	Expires   string `json:"expires"`
}
type SessionOut struct {
	Hash        string `json:"hash"`
	CreatedAt   string `json:"createdAt"`
	Location    string `json:"location,omitempty"`
	OfficialApp bool   `json:"officialApp,omitempty"`
	AppName     string `json:"appName,omitempty"`
	Valid       bool   `json:"valid"`
	Current     bool   `json:"current"`
}
