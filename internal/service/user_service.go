package service

import (
	"database/sql"
	"errors"

	"go-serverhttp-template/internal/dao"
)

var ErrUserNotFound = errors.New("user not found")

type UserService interface {
	GetUser(id int) (*dao.User, error)
}

type userService struct {
	dao dao.UserDAO
}

func NewUserService(d dao.UserDAO) UserService {
	return &userService{dao: d}
}

func (s *userService) GetUser(id int) (*dao.User, error) {
	u, err := s.dao.FindByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}
