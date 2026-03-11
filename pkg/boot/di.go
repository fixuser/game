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

// get 根据类型获取对应的值，内部封装读锁保护
func (c *Container) get(t reflect.Type) reflect.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val := c.values[t]
	if val.IsValid() {
		return val
	}

	if t.Kind() == reflect.Interface {
		for k, v := range c.values {
			if k.Implements(t) {
				val = v
				break
			}
		}
	}
	return val
}

// Get 从容器获取依赖并注入到目标指针中，支持同时获取多个值
// 自动解包指针层级以匹配注册的数据类型。如果目标是指针，会逐级解包直到找到匹配并赋值。
func (c *Container) Get(targets ...any) error {
	for _, target := range targets {
		v := reflect.ValueOf(target)

		// 保证外层传入的是指针且非 nil，否则无法正常注入(Unaddressable)
		if v.Kind() != reflect.Ptr || v.IsNil() {
			return fmt.Errorf("boot: target must be a non-nil pointer, got %T", target)
		}

		isSet := false
		// 逐层解开指针寻找匹配的类型进行注入
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				break // 无法深入解包 nil 指针
			}
			v = v.Elem()
			if value := c.get(v.Type()); value.IsValid() {
				v.Set(value)
				isSet = true
				break
			}
		}

		if !isSet {
			return fmt.Errorf("boot: value not found for type: %v", reflect.TypeOf(target))
		}
	}
	return nil
}
