package types

type Part struct {
	ID   int    `json:"id"`
	Salt string `json:"salt,omitempty"`
}

type Parts = []Part
