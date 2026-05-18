package service

import (
	"context"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type UserService interface {
	GetUser(ctx context.Context, id int) (*model.User, error)
}

type userService struct {
	dao dao.UserDAO
}

func NewUserService(d dao.UserDAO) UserService {
	return &userService{dao: d}
}

func (s *userService) GetUser(ctx context.Context, id int) (*model.User, error) {
	return s.dao.FindByID(ctx, id)
}

type memoryUserService struct {
	users map[int]model.User
}

func NewMemoryUserService() UserService {
	return &memoryUserService{
		users: map[int]model.User{
			1: {ID: 1, Name: "Ada"},
		},
	}
}

func (s *memoryUserService) GetUser(ctx context.Context, id int) (*model.User, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, dao.ErrUserNotFound
	}
	return &user, nil
}
