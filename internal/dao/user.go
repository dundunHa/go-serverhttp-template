package dao

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dundunHa/go-serverhttp-template/internal/db"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

var ErrUserNotFound = pgx.ErrNoRows

type UserDAO interface {
	FindByID(ctx context.Context, id int) (*model.User, error)
	ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error)
}

type userDAO struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewUserDAO(pool *pgxpool.Pool) UserDAO {
	return &userDAO{
		pool:    pool,
		queries: db.New(pool),
	}
}

func (d *userDAO) FindByID(ctx context.Context, id int) (*model.User, error) {
	user, err := d.queries.GetUser(ctx, int64(id))
	if err != nil {
		return nil, err
	}

	return &model.User{
		ID:   int(user.ID),
		Name: user.Name,
	}, nil
}

func (d *userDAO) ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error) {
	if identity.Provider == "" || identity.Subject == "" {
		return nil, ErrUserNotFound
	}

	user, err := getUserInfoByAuthIdentity(ctx, d.queries, identity)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}

	qtx := d.queries.WithTx(tx)
	createdUser, err := qtx.CreateUser(ctx, authDisplayName(identity))
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}

	_, err = qtx.CreateAuthIdentity(ctx, db.CreateAuthIdentityParams{
		Provider:        identity.Provider,
		ProviderSubject: identity.Subject,
		UserID:          createdUser.ID,
		Email:           identity.Email,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		if isUniqueViolation(err) {
			return getUserInfoByAuthIdentity(ctx, d.queries, identity)
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &model.UserInfo{
		ID:              strconv.Itoa(int(createdUser.ID)),
		Email:           identity.Email,
		Provider:        identity.Provider,
		ProviderSubject: identity.Subject,
	}, nil
}

func getUserInfoByAuthIdentity(ctx context.Context, q *db.Queries, identity model.AuthIdentity) (*model.UserInfo, error) {
	user, err := q.GetUserInfoByAuthIdentity(ctx, db.GetUserInfoByAuthIdentityParams{
		Provider:        identity.Provider,
		ProviderSubject: identity.Subject,
	})
	if err != nil {
		return nil, err
	}

	return &model.UserInfo{
		ID:              strconv.Itoa(int(user.ID)),
		Email:           user.Email,
		Provider:        user.Provider,
		ProviderSubject: user.ProviderSubject,
	}, nil
}

func authDisplayName(identity model.AuthIdentity) string {
	if identity.Email != "" {
		return identity.Email
	}
	return identity.Provider + " user"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
