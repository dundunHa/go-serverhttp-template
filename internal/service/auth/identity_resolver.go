package auth

import (
	"context"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type IdentityResolver interface {
	ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error)
}
