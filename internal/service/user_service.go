package service

import "go-serverhttp-template/internal/dao"

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
	return s.dao.FindByID(id)
}
