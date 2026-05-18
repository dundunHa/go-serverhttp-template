package dao

import (
	"context"
	"database/sql"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

var ErrUserNotFound = sql.ErrNoRows

type UserDAO interface {
	FindByID(ctx context.Context, id int) (*model.User, error)
}

type userDAO struct{ db *sql.DB }

func NewUserDAO(db *sql.DB) UserDAO {
	return &userDAO{db: db}
}

func (d *userDAO) FindByID(ctx context.Context, id int) (*model.User, error) {
	u := &model.User{}
	err := d.db.QueryRowContext(ctx, "SELECT id,name FROM users WHERE id=$1", id).
		Scan(&u.ID, &u.Name)

	return u, err
}
