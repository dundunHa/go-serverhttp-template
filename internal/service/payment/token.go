package payment

import (
	"context"
	"errors"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// ErrAccountTokenNotFound 在 service 层暴露 dao 同名错误，便于 api 层用 errors.Is 判断。
var ErrAccountTokenNotFound = dao.ErrAccountTokenNotFound

// TokenDAO 是 TokenService 的持久化依赖。具体实现由 internal/dao 提供。
type TokenDAO interface {
	GetOrCreateAccountToken(ctx context.Context, userID int64) (string, error)
	GetAccountTokenByToken(ctx context.Context, token string) (model.AppleAccountToken, error)
}

// TokenService 负责 user_id ↔ Apple appAccountToken UUID 的稳定映射。
//
// 该 token 不是认证凭证；它只是本服务与 Apple transaction 之间的关联键。
// 对应 API endpoint 仍然要求调用方携带 Bearer access token。
type TokenService struct {
	dao TokenDAO
}

// NewTokenService 构造 TokenService。dao 为 nil 时 service 调用都会返回 not-configured 错误。
func NewTokenService(d TokenDAO) *TokenService {
	return &TokenService{dao: d}
}

// EnsureAccountToken 返回 userID 对应的 appAccountToken UUID，首调时持久化生成。
func (s *TokenService) EnsureAccountToken(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.dao == nil {
		return "", errors.New("payment: token service not configured")
	}
	return s.dao.GetOrCreateAccountToken(ctx, userID)
}

// ResolveUserByToken 通过 token 反查映射，未命中返回 ErrAccountTokenNotFound。
//
// webhook / verify 路径用它把 Apple transaction 的 appAccountToken 映射到本地 user_id；
// 不存在时由调用方决定走 PENDING_USER_BINDING 还是 verify 400 mismatch。
func (s *TokenService) ResolveUserByToken(ctx context.Context, token string) (model.AppleAccountToken, error) {
	if s == nil || s.dao == nil {
		return model.AppleAccountToken{}, errors.New("payment: token service not configured")
	}
	if token == "" {
		return model.AppleAccountToken{}, ErrAccountTokenNotFound
	}
	return s.dao.GetAccountTokenByToken(ctx, token)
}
