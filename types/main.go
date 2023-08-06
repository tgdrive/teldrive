package types

import "github.com/gotd/td/tg"

type AppError struct {
	Error error
	Code  int
}

type Part struct {
	Location *tg.InputDocumentFileLocation
	Size     int64
	Start    int64
	End      int64
}
