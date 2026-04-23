package auth

import (
	"context"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// Params defines the auth handler input extracted from request headers.
type Params struct {
	XProjectID string `header:"X-Project-ID"`
}

// Data holds the authenticated project context available to all auth endpoints.
type Data struct {
	ProjectID string
}

// AuthHandler validates the X-Project-ID header and returns auth data.
//
//encore:authhandler
func AuthHandler(ctx context.Context, params *Params) (auth.UID, *Data, error) {
	if params.XProjectID == "" {
		return "", nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "missing X-Project-ID header",
		}
	}

	return auth.UID(params.XProjectID), &Data{ProjectID: params.XProjectID}, nil
}
