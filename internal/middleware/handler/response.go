package handler

type Response struct {
	StatusCode int
	Data       interface{}
	Err        error
}

func NewSuccessResponse(statusCode int, data interface{}) *Response {
	return &Response{
		StatusCode: statusCode,
		Data:       data,
	}
}

func NewErrorResponse(statusCode int, code ErrorCode, message string, details interface{}) *Response {
	return &Response{
		StatusCode: statusCode,
		Err: &ErrorResponse{
			Code:    code,
			Message: message,
			Errors:  details,
		},
	}
}

func NewInternalErrorResponse(err error) *Response {
	return &Response{
		Err: err,
	}
}
