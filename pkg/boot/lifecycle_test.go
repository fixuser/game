package boot

import (
	"context"
	"errors"
	"testing"
)

// mockService 是用于测试的 Service 模拟实现
type mockService struct {
	name      string
	loadErr   error
	unloadErr error
	loaded    bool
	unloaded  bool
	loadSeq   *[]string
	unloadSeq *[]string
}

func (m *mockService) Name() string { return m.name }

func (m *mockService) Load(_ context.Context) error {
	if m.loadErr != nil {
		return m.loadErr
	}
	m.loaded = true
	if m.loadSeq != nil {
		*m.loadSeq = append(*m.loadSeq, m.name)
	}
	return nil
}

func (m *mockService) Unload(_ context.Context) error {
	m.unloaded = true
	if m.unloadSeq != nil {
		*m.unloadSeq = append(*m.unloadSeq, m.name)
	}
	return m.unloadErr
}

// TestLifecycleLoadOrder 测试服务按注册顺序加载
func TestLifecycleLoadOrder(t *testing.T) {
	var seq []string
	b := NewBoot()
	b.Register(
		&mockService{name: "a", loadSeq: &seq},
		&mockService{name: "b", loadSeq: &seq},
		&mockService{name: "c", loadSeq: &seq},
	)

	if err := b.Load(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"a", "b", "c"}
	if len(seq) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, seq)
	}
	for i, v := range expected {
		if seq[i] != v {
			t.Fatalf("expected %v, got %v", expected, seq)
		}
	}
}

// TestLifecycleUnloadReverseOrder 测试服务按逆序卸载
func TestLifecycleUnloadReverseOrder(t *testing.T) {
	var loadSeq, unloadSeq []string
	b := NewBoot()
	b.Register(
		&mockService{name: "a", loadSeq: &loadSeq, unloadSeq: &unloadSeq},
		&mockService{name: "b", loadSeq: &loadSeq, unloadSeq: &unloadSeq},
		&mockService{name: "c", loadSeq: &loadSeq, unloadSeq: &unloadSeq},
	)

	if err := b.Load(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b.Unload(context.Background())

	expected := []string{"c", "b", "a"}
	if len(unloadSeq) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, unloadSeq)
	}
	for i, v := range expected {
		if unloadSeq[i] != v {
			t.Fatalf("expected %v, got %v", expected, unloadSeq)
		}
	}
}

// TestLifecycleLoadFailRollback 测试加载失败时自动回滚已加载的服务
func TestLifecycleLoadFailRollback(t *testing.T) {
	var loadSeq, unloadSeq []string
	errFail := errors.New("load failed")

	svcA := &mockService{name: "a", loadSeq: &loadSeq, unloadSeq: &unloadSeq}
	svcB := &mockService{name: "b", loadSeq: &loadSeq, unloadSeq: &unloadSeq}
	svcC := &mockService{name: "c", loadErr: errFail, loadSeq: &loadSeq, unloadSeq: &unloadSeq}

	b := NewBoot()
	b.Register(svcA, svcB, svcC)

	err := b.Load(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errFail) {
		t.Fatalf("expected errFail, got %v", err)
	}

	// a 和 b 应被加载
	if !svcA.loaded || !svcB.loaded {
		t.Fatal("expected a and b to be loaded")
	}
	// c 不应被加载
	if svcC.loaded {
		t.Fatal("expected c NOT to be loaded")
	}

	// 回滚应逆序卸载 b, a
	expectedUnload := []string{"b", "a"}
	if len(unloadSeq) != len(expectedUnload) {
		t.Fatalf("expected unload %v, got %v", expectedUnload, unloadSeq)
	}
	for i, v := range expectedUnload {
		if unloadSeq[i] != v {
			t.Fatalf("expected unload %v, got %v", expectedUnload, unloadSeq)
		}
	}
}

// TestLifecycleRegisterDeduplicate 测试 Register 去重，同名服务只保留第一个
func TestLifecycleRegisterDeduplicate(t *testing.T) {
	var seq []string
	b := NewBoot()
	b.Register(
		&mockService{name: "a", loadSeq: &seq},
		&mockService{name: "b", loadSeq: &seq},
		&mockService{name: "a", loadSeq: &seq},
	)

	if err := b.Load(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"a", "b"}
	if len(seq) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, seq)
	}
	for i, v := range expected {
		if seq[i] != v {
			t.Fatalf("expected %v, got %v", expected, seq)
		}
	}
}

// TestBootSetGet 测试 Boot 的 Set/Get 依赖注入集成
func TestBootSetGet(t *testing.T) {
	b := NewBoot()
	b.Set(42, "hello")

	var (
		gotInt int
		gotStr string
	)
	if err := b.Get(&gotInt, &gotStr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotInt != 42 {
		t.Fatalf("expected 42, got %d", gotInt)
	}
	if gotStr != "hello" {
		t.Fatalf("expected 'hello', got '%s'", gotStr)
	}
}

// TestLifecycleUnloadContinuesOnError 测试卸载时即使某个服务失败也继续卸载其余服务
func TestLifecycleUnloadContinuesOnError(t *testing.T) {
	var unloadSeq []string
	b := NewBoot()
	b.Register(
		&mockService{name: "a", unloadSeq: &unloadSeq},
		&mockService{name: "b", unloadErr: errors.New("fail"), unloadSeq: &unloadSeq},
		&mockService{name: "c", unloadSeq: &unloadSeq},
	)

	_ = b.Load(context.Background())
	b.Unload(context.Background())

	// 即使 b 失败，c、b、a 都应被调用 Unload
	expected := []string{"c", "b", "a"}
	if len(unloadSeq) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, unloadSeq)
	}
	for i, v := range expected {
		if unloadSeq[i] != v {
			t.Fatalf("expected %v, got %v", expected, unloadSeq)
		}
	}
}
