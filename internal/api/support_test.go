package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

func TestMapServiceErrorContextLifecycle(t *testing.T) {
	t.Parallel()

	if err := mapServiceError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}

	err := mapServiceError(context.DeadlineExceeded)
	var problem *Problem
	if !errors.As(err, &problem) {
		t.Fatalf("deadline error = %T, want *Problem", err)
	}
	if problem.Status != http.StatusGatewayTimeout || problem.Code != "request_timeout" {
		t.Fatalf("deadline problem = %#v", problem)
	}
}

func TestParseRangeResolvesFullContentLength(t *testing.T) {
	t.Parallel()

	got, partial, err := parseRange(gen.OptString{}, 557123)
	if err != nil {
		t.Fatalf("parseRange() error = %v", err)
	}
	if partial || got.Offset != 0 || got.Length != 557123 {
		t.Fatalf("parseRange() = (%+v, %t), want full content", got, partial)
	}
}

func TestContentDisposition(t *testing.T) {
	t.Parallel()

	if got := contentDisposition("report 2026.pdf", false); got != `inline; filename="report 2026.pdf"` {
		t.Fatalf("inline disposition = %q", got)
	}
	if got := contentDisposition("report 2026.pdf", true); got != `attachment; filename="report 2026.pdf"` {
		t.Fatalf("attachment disposition = %q", got)
	}
}

func TestErrorHandlerIgnoresClientCancellation(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
	ErrorHandler(request.Context(), response, request, context.Canceled)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want untouched recorder status %d", response.Code, http.StatusOK)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
}
