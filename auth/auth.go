package auth

import (
	"context"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

var secrets struct {
	ClientSecretKey string
}

// AuthData is attached to every authenticated request.
// OrgID is the active Clerk organisation — used as the tenant filter on every DB query.
type AuthData struct {
	OrgID string
	Role  string
}

// VerifyToken validates the Clerk JWT and enforces that the session
// has an active organisation. Personal (non-org) sessions are rejected.
//
//encore:authhandler
func VerifyToken(ctx context.Context, token string) (auth.UID, *AuthData, error) {
	clerk.SetKey(secrets.ClientSecretKey)

	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{Token: token})
	if err != nil {
		return "", nil, errs.B().
			Code(errs.Unauthenticated).
			Msg("invalid or expired token").
			Err()
	}

	if claims.ActiveOrganizationID == "" {
		return "", nil, errs.B().
			Code(errs.PermissionDenied).
			Msg("no active organisation on this session — please select an organisation in the app").
			Err()
	}

	return auth.UID(claims.Subject), &AuthData{
		OrgID: claims.ActiveOrganizationID,
		Role:  claims.ActiveOrganizationRole,
	}, nil
}
