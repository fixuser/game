package boot

import (
	"fmt"
	"reflect"
	"sync"
)

// Container 是基于反射的依赖注入容器，支持按类型存取依赖
type Container struct {
	mu     sync.RWMutex
	values map[reflect.Type]reflect.Value
}

// NewContainer 创建并返回一个初始化好的依赖注入容器
func NewContainer() *Container {
	return &Container{
		values: make(map[reflect.Type]reflect.Value),
	}
}

// Set 注册依赖到容器，支持同时注册多个值
// 值的实际类型作为 key 存储，后续可通过相同类型或兼容的接口类型获取
func (c *Container) Set(values ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, v := range values {
		c.values[reflect.TypeOf(v)] = reflect.ValueOf(v)
	}
}

// Get 从容器获取依赖并注入到目标指针中，支持同时获取多个值
// targets 中的每一项都必须是指针类型，容器会根据指针指向的类型查找匹配的依赖
// 支持接口匹配：Set 具体实现后，可以通过接口类型的指针 Get 获取
func (c *Container) Get(targets ...any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, t := range targets {
		rv := reflect.ValueOf(t)
		if rv.Kind() != reflect.Ptr {
			return fmt.Errorf("boot: target must be a pointer, got %T", t)
		}
		elemType := rv.Elem().Type()

		// 精确匹配
		if val, ok := c.values[elemType]; ok {
			rv.Elem().Set(val)
			continue
		}

		// 接口匹配：目标是接口类型时，查找实现了该接口的值
		if elemType.Kind() == reflect.Interface {
			found := false
			for _, val := range c.values {
				if val.Type().Implements(elemType) {
					rv.Elem().Set(val)
					found = true
					break
				}
			}
			if found {
				continue
			}
		}

		return fmt.Errorf("boot: type %s not registered", elemType)
	}
	return nil
}
