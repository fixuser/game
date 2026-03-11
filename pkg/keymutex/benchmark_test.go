package keymutex

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkKeyMutex_SingleKey 高并发场景下对同一个 key 加锁解锁（最高竞争）
func BenchmarkKeyMutex_SingleKey(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	key := 42

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkKeyMutex_RandomKeys 中等竞争：多个 goroutine 随机访问一定数量的 keys
func BenchmarkKeyMutex_RandomKeys(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	numKeys := 100 // 限制在 100 个 key，确保存在一定的冲突率

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 为了不让 rand.Intn 内部的全局锁影响性能，使用局部随机数生成器
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			key := r.Intn(numKeys)
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkKeyMutex_NoConflict 无竞争场景：每个 goroutine 操作自己独占的 keys
func BenchmarkKeyMutex_NoConflict(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	var gCounter int32

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 每个 goroutine 获取一段独占的 key 范围
		base := int(atomic.AddInt32(&gCounter, 1)) * 10000
		i := 0
		for pb.Next() {
			key := base + (i % 1000)
			i++
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkMultiKeyLocker 测试获取多个 keys（存在竞争）
func BenchmarkMultiKeyLocker(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	numKeys := 50 // 较少的 key 集合，保证有死锁检测/避让的压力

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			// 每个 goroutine 随机拿 3 个 key
			k1 := r.Intn(numKeys)
			k2 := r.Intn(numKeys)
			k3 := r.Intn(numKeys)

			locker := km.Locker(ctx, k1, k2, k3)
			if err := locker.Lock(ctx); err == nil {
				locker.Unlock(ctx)
			}
		}
	})
}

// BenchmarkSyncMutex_Compare 对比：标准的 sync.Mutex 的性能（最高竞争基准线）
func BenchmarkSyncMutex_Compare(b *testing.B) {
	var mu sync.Mutex
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

// BenchmarkSyncMap_Compare 对比：如果只用 sync.Map + sync.Mutex 组合的简单实现性能
// 这是一种常见的（但内存泄漏的）实现方式，用来对比说明 KeyMutex 的并发机制优势
func BenchmarkSyncMap_Compare(b *testing.B) {
	var m sync.Map

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			key := r.Intn(100)
			v, _ := m.LoadOrStore(key, &sync.Mutex{})
			mu := v.(*sync.Mutex)
			mu.Lock()
			mu.Unlock()
		}
	})
}
