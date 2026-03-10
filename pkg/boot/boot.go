package boot

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
)

// Boot 是服务生命周期管理器，负责 Service 的注册、按序加载、逆序卸载和依赖注入
type Boot struct {
	mu       sync.RWMutex
	services []Service
	names    map[string]struct{}
	di       *Container
}

// NewBoot 创建并返回一个初始化好的服务管理器
func NewBoot() *Boot {
	return &Boot{
		names: make(map[string]struct{}),
		di:    NewContainer(),
	}
}

// Set 注册依赖到容器，供后续通过 Get 获取
func (b *Boot) Set(values ...any) {
	b.di.Set(values...)
}

// Get 从容器获取依赖注入到目标指针，targets 必须是指针类型
func (b *Boot) Get(targets ...any) error {
	return b.di.Get(targets...)
}

// Register 注册服务，按注册顺序追加，自动按 Name() 全局去重
func (b *Boot) Register(services ...Service) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, p := range services {
		if _, ok := b.names[p.Name()]; ok {
			continue
		}
		b.names[p.Name()] = struct{}{}
		b.services = append(b.services, p)
	}
}

// unloadServices 逆序卸载 services[0:count] 的服务，即使个别失败也继续
func (b *Boot) unloadServices(ctx context.Context, count int) {
	for i := count - 1; i >= 0; i-- {
		s := b.services[i]
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("unloading service")
		if err := s.Unload(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Str("service", s.Name()).Msg("failed to unload service")
		}
	}
}

// Load 按注册顺序加载所有服务，若某个服务加载失败则逆序卸载已加载的服务并返回错误
func (b *Boot) Load(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.services {
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("loading service")
		if err := s.Load(ctx); err != nil {
			log.Ctx(ctx).Error().Err(err).Str("service", s.Name()).Msg("failed to load service")
			b.unloadServices(ctx, i)
			return err
		}
		log.Ctx(ctx).Info().Str("service", s.Name()).Msg("service loaded")
	}
	return nil
}

// Unload 逆序卸载所有已加载的服务，即使个别服务卸载失败也会继续卸载其余服务
func (b *Boot) Unload(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.unloadServices(ctx, len(b.services))
}
