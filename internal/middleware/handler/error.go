package handler

import (
	"encoding/json"
	"fmt"
)

type ErrorCode string

const (
	// 400 bad request
	InvalidQueryValue = ErrorCode("InvalidQueryValue")
	InvalidUriValue   = ErrorCode("InvalidUriValue")
	InvalidBodyValue  = ErrorCode("InvalidBodyValue")

	// 404 not found
	NotFoundEntity = ErrorCode("NotFoundEntity")

	// 409 duplicate
	DuplicateEntry = ErrorCode("DuplicateEntry")

	// 500
	InternalServerError = ErrorCode("InternalServerError")
)

type ErrorResponse struct {
	Code    ErrorCode   `json:"code"`
	Message string      `json:"message"`
	Errors  interface{} `json:"-"`
}

func (e *ErrorResponse) MarshalJSON() ([]byte, error) {
	message := fmt.Sprintf("[%s]", e.Code)
	if e.Message != "" {
		message += " " + e.Message
	}
	m := map[string]interface{}{
		"code":    e.Code,
		"message": message,
	}
	if e.Errors != nil {
		m["errors"] = e.Errors
	}
	return json.Marshal(&m)
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("ErrorResponse{Code:%s, Message:%s, Errors:%v}", e.Code, e.Message, e.Errors)
}
