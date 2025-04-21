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
	// 这里可加业务校验、缓存、事务等
	return s.dao.FindByID(id)
}
