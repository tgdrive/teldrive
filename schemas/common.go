package schemas

type Message struct {
	Status  bool   `json:"status"`
	Message string `json:"message,omitempty"`
}
