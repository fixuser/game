package boot

import "context"

// defaultBoot 是包级别的默认全局实例
var defaultBoot = NewBoot()

// ---------- DI 全局方法 ----------

// Set 注册全局依赖
func Set(values ...any) {
	defaultBoot.Set(values...)
}

// Get 获取全局依赖
func Get(targets ...any) error {
	return defaultBoot.Get(targets...)
}

// ---------- PubSub 全局方法 ----------

// Publish 向指定 topic 发布消息
func Publish(ctx context.Context, topic string, args ...any) {
	defaultBoot.Publish(ctx, topic, args...)
}

// Subscribe 订阅指定 topic 的消息
func Subscribe(topic string, handler any, opts ...SubOption) *Subscriber {
	return defaultBoot.Subscribe(topic, handler, opts...)
}

// Close 关闭全局管理器的所有发布订阅及其他资源（如适用）
func Close() {
	defaultBoot.PubSub.Close()
}

// ---------- Lifecycle 全局方法 ----------

// Register 注册服务到全局管理器
func Register(services ...Service) {
	defaultBoot.Register(services...)
}

// Load 加载全局管理器中的所有服务
func Load(ctx context.Context) error {
	return defaultBoot.Load(ctx)
}

// Unload 卸载全局管理器中的所有服务
func Unload(ctx context.Context) {
	defaultBoot.Unload(ctx)
}
