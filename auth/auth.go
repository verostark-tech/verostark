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

//encore:service
type Service struct{}

// initService sets the Clerk API key once at startup.
func initService() (*Service, error) {
	clerk.SetKey(secrets.ClientSecretKey)
	return &Service{}, nil
}

// VerifyToken validates the Clerk JWT and enforces that the session
// has an active organisation. Personal (non-org) sessions are rejected.
//
//encore:authhandler
func (s *Service) VerifyToken(ctx context.Context, token string) (auth.UID, *AuthData, error) {
	claims, err := jwt.Verify(ctx, &jwt.VerifyParams{Token: token})
	if err != nil {
		return "", nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "invalid or expired token",
		}
	}

	if claims.ActiveOrganizationID == "" {
		return "", nil, &errs.Error{
			Code:    errs.PermissionDenied,
			Message: "no active organisation on this session — please select an organisation in the app",
		}
	}

	return auth.UID(claims.Subject), &AuthData{
		OrgID: claims.ActiveOrganizationID,
		Role:  claims.ActiveOrganizationRole,
	}, nil
}
