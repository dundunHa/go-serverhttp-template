package service

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

var ErrAuthIdentityUnsupported = errors.New("auth identity resolution unsupported")

type UserService interface {
	GetUser(ctx context.Context, id int) (*model.User, error)
	ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error)
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

func (s *userService) ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error) {
	_ = ctx
	_ = identity
	return nil, ErrAuthIdentityUnsupported
}

type authIdentityKey struct {
	provider string
	subject  string
}

type memoryUserService struct {
	mu             sync.RWMutex
	users          map[int]model.User
	authIdentities map[authIdentityKey]int
	nextAuthUserID int
}

func NewMemoryUserService() UserService {
	return &memoryUserService{
		users: map[int]model.User{
			1: {ID: 1, Name: "Ada"},
		},
		authIdentities: make(map[authIdentityKey]int),
		nextAuthUserID: 1,
	}
}

func (s *memoryUserService) GetUser(ctx context.Context, id int) (*model.User, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return nil, dao.ErrUserNotFound
	}
	return &user, nil
}

func (s *memoryUserService) ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error) {
	_ = ctx
	if identity.Provider == "" || identity.Subject == "" {
		return nil, dao.ErrUserNotFound
	}

	key := authIdentityKey{
		provider: identity.Provider,
		subject:  identity.Subject,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	userID, ok := s.authIdentities[key]
	if !ok {
		userID = s.nextAuthUserID
		s.nextAuthUserID++
		s.authIdentities[key] = userID
		if _, exists := s.users[userID]; !exists {
			s.users[userID] = model.User{ID: userID, Name: authDisplayName(identity)}
		}
	}

	return &model.UserInfo{
		ID:              strconv.Itoa(userID),
		Email:           identity.Email,
		Provider:        identity.Provider,
		ProviderSubject: identity.Subject,
	}, nil
}

func authDisplayName(identity model.AuthIdentity) string {
	if identity.Email != "" {
		return identity.Email
	}
	return identity.Provider + " user"
}
