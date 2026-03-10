package boot

// Boot 是服务生命周期管理器
// 组合 Container（依赖注入）、PubSub（发布订阅）和 Lifecycle（服务生命周期）
type Boot struct {
	*Container
	*PubSub
	*Lifecycle
}

// NewBoot 创建并返回一个初始化好的服务管理器
func NewBoot() *Boot {
	return &Boot{
		Container: NewContainer(),
		PubSub:    NewPubSub(),
		Lifecycle: NewLifecycle(),
	}
}
