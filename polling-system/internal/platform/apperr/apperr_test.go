package apperr

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppError_ErrorPrefersMessageThenCodeThenUnderlying(t *testing.T) {
	base := errors.New("boom")
	e := &AppError{Code: "code", Message: "msg", Err: base}
	if got := e.Error(); got != "msg" {
		t.Fatalf("expected msg, got %q", got)
	}

	e.Message = ""
	if got := e.Error(); got != "code" {
		t.Fatalf("expected code, got %q", got)
	}

	e.Code = ""
	if got := e.Error(); got != "boom" {
		t.Fatalf("expected underlying error, got %q", got)
	}
}

func TestAppError_NilReceiverIsSafe(t *testing.T) {
	var e *AppError
	if got := e.Error(); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := e.Unwrap(); got != nil {
		t.Fatalf("expected nil unwrap, got %v", got)
	}
	if got := e.StatusCode(); got != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", got)
	}
}

func TestFromError_WrapsNonAppError(t *testing.T) {
	base := errors.New("boom")
	e := FromError(base)
	if e == nil {
		t.Fatalf("expected non-nil error")
	}
	if e.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %q", e.Code)
	}
	if e.Message != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("unexpected message: %q", e.Message)
	}
	if !errors.Is(e, base) {
		t.Fatalf("expected wrapped error")
	}
	if e.StatusCode() != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", e.StatusCode())
	}
}

func TestFromError_PreservesAppError(t *testing.T) {
	base := errors.New("boom")
	want := BadRequest("bad", "nope", base)
	got := FromError(want)
	if got != want {
		t.Fatalf("expected same instance")
	}
}

