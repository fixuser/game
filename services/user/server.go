package user

import (
	"context"
	"sync"

	"github.com/game/game/apis/userpb"
)

type userServer struct {
	userpb.UnimplementedUserServerServer
	mu       sync.RWMutex
	wg       sync.WaitGroup
	name     string
	exitChan chan struct{}
}

var UserService = newUserServer()

func newUserServer() *userServer {
	return &userServer{
		name:     "game.user",
		exitChan: make(chan struct{}),
	}
}

func (s *userServer) Name() string {
	return s.name
}

func (s *userServer) Load(ctx context.Context) error {
	return nil
}

func (s *userServer) Unload(ctx context.Context) error {
	s.mu.Lock()
	close(s.exitChan)
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}
