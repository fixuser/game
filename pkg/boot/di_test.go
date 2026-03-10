package boot

import (
	"fmt"
	"testing"
)

// 测试用自定义结构体
type testUser struct {
	Name string
	Age  int
}

// 测试用接口
type testGreeter interface {
	Greet() string
}

// 测试用接口实现
type testGreeterImpl struct {
	Prefix string
}

func (g *testGreeterImpl) Greet() string {
	return fmt.Sprintf("%s, hello!", g.Prefix)
}

// TestContainerSetGetInt 测试 int 类型的注入和获取
func TestContainerSetGetInt(t *testing.T) {
	c := NewContainer()
	val := 42
	c.Set(val)

	var got int
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

// TestContainerSetGetString 测试 string 类型的注入和获取
func TestContainerSetGetString(t *testing.T) {
	c := NewContainer()
	c.Set("hello")

	var got string
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected 'hello', got '%s'", got)
	}
}

// TestContainerSetGetBool 测试 bool 类型的注入和获取
func TestContainerSetGetBool(t *testing.T) {
	c := NewContainer()
	c.Set(true)

	var got bool
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("expected true, got false")
	}
}

// TestContainerSetGetFloat64 测试 float64 类型的注入和获取
func TestContainerSetGetFloat64(t *testing.T) {
	c := NewContainer()
	c.Set(3.14)

	var got float64
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3.14 {
		t.Fatalf("expected 3.14, got %f", got)
	}
}

// TestContainerSetGetStruct 测试自定义结构体值类型的注入和获取
func TestContainerSetGetStruct(t *testing.T) {
	c := NewContainer()
	u := testUser{Name: "Alice", Age: 30}
	c.Set(u)

	var got testUser
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Fatalf("expected {Alice 30}, got %+v", got)
	}
}

// TestContainerSetGetStructPointer 测试自定义结构体指针类型的注入和获取
func TestContainerSetGetStructPointer(t *testing.T) {
	c := NewContainer()
	u := &testUser{Name: "Bob", Age: 25}
	c.Set(u)

	var got *testUser
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Bob" || got.Age != 25 {
		t.Fatalf("expected {Bob 25}, got %+v", got)
	}
}

// TestContainerSetGetInterface 测试接口类型的注入和获取：Set 具体实现，Get 接口类型
func TestContainerSetGetInterface(t *testing.T) {
	c := NewContainer()
	impl := &testGreeterImpl{Prefix: "Hi"}
	c.Set(impl)

	var got testGreeter
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Greet() != "Hi, hello!" {
		t.Fatalf("expected 'Hi, hello!', got '%s'", got.Greet())
	}
}

// TestContainerSetGetMultiple 测试同时注入和获取多个不同类型的值
func TestContainerSetGetMultiple(t *testing.T) {
	c := NewContainer()
	c.Set(100, "world", true)

	var (
		gotInt  int
		gotStr  string
		gotBool bool
	)
	if err := c.Get(&gotInt, &gotStr, &gotBool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotInt != 100 {
		t.Fatalf("expected 100, got %d", gotInt)
	}
	if gotStr != "world" {
		t.Fatalf("expected 'world', got '%s'", gotStr)
	}
	if !gotBool {
		t.Fatal("expected true, got false")
	}
}

// TestContainerGetNotRegistered 测试获取未注册类型时应返回 error
func TestContainerGetNotRegistered(t *testing.T) {
	c := NewContainer()
	c.Set("registered")

	var got int
	if err := c.Get(&got); err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}

// TestContainerGetNonPointer 测试 Get 参数非指针时应返回 error
func TestContainerGetNonPointer(t *testing.T) {
	c := NewContainer()
	c.Set(42)

	err := c.Get(42)
	if err == nil {
		t.Fatal("expected error for non-pointer target, got nil")
	}
}

// TestContainerOverwrite 测试重复 Set 同类型值时，后者覆盖前者
func TestContainerOverwrite(t *testing.T) {
	c := NewContainer()
	c.Set(1)
	c.Set(2)

	var got int
	if err := c.Get(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected 2 (overwritten), got %d", got)
	}
}

// TestContainerInterfaceNotFound 测试获取未实现的接口类型时应返回 error
func TestContainerInterfaceNotFound(t *testing.T) {
	c := NewContainer()
	c.Set("not_a_greeter")

	var got testGreeter
	if err := c.Get(&got); err == nil {
		t.Fatal("expected error for unimplemented interface, got nil")
	}
}
