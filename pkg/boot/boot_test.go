package boot

import (
	"context"
	"testing"
)

func TestBoot_Context(t *testing.T) {
	b := NewBoot()
	ctx := b.Context(context.Background())

	// 从 ctx 中取出的 Boot 应与原始实例一致
	got := FromContext(ctx)
	if got != b {
		t.Fatalf("FromContext 返回的 Boot 实例不一致: got=%p, want=%p", got, b)
	}
}

func TestFromContext_Fallback(t *testing.T) {
	// 空 ctx 中不存在 Boot，应回退返回全局 GetBoot()
	got := FromContext(context.Background())
	if got != GetBoot() {
		t.Fatalf("预期返回 GetBoot(), 实际返回: %+v", got)
	}
}

func TestBoot_Context_Overwrite(t *testing.T) {
	b1 := NewBoot()
	b2 := NewBoot()

	ctx := b1.Context(context.Background())
	ctx = b2.Context(ctx)

	// 后写入的 Boot 应覆盖前一个
	got := FromContext(ctx)
	if got != b2 {
		t.Fatalf("多次写入后，FromContext 应返回最后写入的实例: got=%p, want=%p", got, b2)
	}
}

func TestBoot_Context_PreservesParent(t *testing.T) {
	type testKey struct{}
	parent := context.WithValue(context.Background(), testKey{}, "hello")

	b := NewBoot()
	ctx := b.Context(parent)

	// Boot 应能正确取出
	got := FromContext(ctx)
	if got != b {
		t.Fatal("FromContext 返回 nil 或不匹配")
	}

	// 父 context 中的值应保留
	v, ok := ctx.Value(testKey{}).(string)
	if !ok || v != "hello" {
		t.Fatalf("父 context 的值丢失: got=%q", v)
	}
}

func TestBoot_Context_Components(t *testing.T) {
	b := NewBoot()
	ctx := b.Context(context.Background())

	got := FromContext(ctx)

	// 验证组合的组件均可访问
	if got.Container == nil {
		t.Fatal("Container 为 nil")
	}
	if got.PubSub == nil {
		t.Fatal("PubSub 为 nil")
	}
	if got.Lifecycle == nil {
		t.Fatal("Lifecycle 为 nil")
	}
}
