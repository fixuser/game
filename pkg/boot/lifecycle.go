package boot

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
)

// Lifecycle 是服务生命周期管理器
// 负责 Service 的注册、按序加载、逆序卸载
type Lifecycle struct {
	mu       sync.RWMutex
	services []Service
	names    map[string]struct{}
}

// NewLifecycle 创建并返回一个初始化好的生命周期管理器
func NewLifecycle() *Lifecycle {
	return &Lifecycle{
		names: make(map[string]struct{}),
	}
}

// Register 注册服务，按注册顺序追加，自动按 Name() 全局去重
func (lc *Lifecycle) Register(services ...Service) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for _, p := range services {
		if _, ok := lc.names[p.Name()]; ok {
			continue
		}
		lc.names[p.Name()] = struct{}{}
		lc.services = append(lc.services, p)
	}
}

// unloadServices 逆序卸载 services[0:count] 的服务，即使个别失败也继续
func (lc *Lifecycle) unloadServices(ctx context.Context, count int) {
	for i := count - 1; i >= 0; i-- {
		s := lc.services[i]
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("unloading service")
		if err := s.Unload(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Str("service", s.Name()).Msg("failed to unload service")
		}
	}
}

// Load 按注册顺序加载所有服务，若某个服务加载失败则逆序卸载已加载的服务并返回错误
func (lc *Lifecycle) Load(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for i, s := range lc.services {
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("loading service")
		if err := s.Load(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Str("service", s.Name()).Msg("failed to load service")
			lc.unloadServices(ctx, i)
			return err
		}
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("service loaded")
	}
	return nil
}

// Unload 逆序卸载所有已加载的服务，即使个别服务卸载失败也会继续卸载其余服务
func (lc *Lifecycle) Unload(ctx context.Context) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.unloadServices(ctx, len(lc.services))
}
