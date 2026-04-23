package auth

import (
	"context"
	"errors"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

func TestAuthHandler_ValidProjectID(t *testing.T) {
	uid, data, err := AuthHandler(context.Background(), &Params{
		XProjectID: "proj-abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != auth.UID("proj-abc123") {
		t.Errorf("expected UID %q, got %q", "proj-abc123", uid)
	}
	if data.ProjectID != "proj-abc123" {
		t.Errorf("expected ProjectID %q, got %q", "proj-abc123", data.ProjectID)
	}
}

func TestAuthHandler_MissingProjectID(t *testing.T) {
	_, _, err := AuthHandler(context.Background(), &Params{
		XProjectID: "",
	})
	if err == nil {
		t.Fatal("expected error for missing X-Project-ID")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if e.Code != errs.InvalidArgument {
		t.Errorf("expected code %v, got %v", errs.InvalidArgument, e.Code)
	}
}
