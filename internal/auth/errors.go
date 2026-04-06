package auth

import "errors"

var (
	ErrAuthTokenExpired      = errors.New("auth.token_expired")
	ErrAuthTokenInvalid      = errors.New("auth.token_invalid")
	ErrAuthSessionInvalid    = errors.New("auth.session_invalid")
	ErrAuthAPIKeyInvalid     = errors.New("auth.api_key_invalid")
	ErrAuthAPIKeySessionMiss = errors.New("auth.api_key_session_missing")
)
