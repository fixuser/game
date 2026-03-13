package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/game/game/pkg/meta"
)

// newTestRedis 创建 miniredis 实例和 Redis 客户端
func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// newTestLimiter 创建带有固定 reload 的限速器（测试用）
func newTestLimiter(t *testing.T, rdb redis.UniversalClient, opts ...Option) *RateLimiter {
	t.Helper()
	// 使用很长的 reload 周期避免测试中自动刷新干扰
	opts = append([]Option{WithReloadPeriod(1 * time.Hour)}, opts...)
	rl := NewRateLimiter(context.Background(), rdb, opts...)
	t.Cleanup(func() { rl.Close() })
	return rl
}

// buildCtx 构建带有 path、method 及其他 meta key 的 context
func buildCtx(path, method string, extras ...string) context.Context {
	m := meta.NewMeta()
	m.Set(meta.MetaRequestPath, path)
	m.Set(meta.MetaRequestMethod, method)
	for i := 0; i+1 < len(extras); i += 2 {
		m.Set(extras[i], extras[i+1])
	}
	return m.Context(context.Background())
}

// TestBasicRateLimit 测试单规则超额被拦截
func TestBasicRateLimit(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	err := rl.SetRule(context.Background(), Rule{
		Name:     "basic",
		Key:      "ip",
		Path:     "/api/test",
		Total:    3,
		Duration: "1m",
	})
	if err != nil {
		t.Fatalf("set rule failed: %v", err)
	}

	ctx := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")

	// 前 3 次应放行
	for i := 0; i < 3; i++ {
		r := rl.Allow(ctx)
		if !r.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if r.Remaining != int64(2-i) {
			t.Fatalf("request %d: expected remaining=%d, got=%d", i+1, 2-i, r.Remaining)
		}
	}

	// 第 4 次应被拦截
	r := rl.Allow(ctx)
	if r.Allowed {
		t.Fatal("4th request should be blocked")
	}
}

// TestMultipleRulesStrictest 测试多规则匹配时返回最严格的
func TestMultipleRulesStrictest(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	// 宽松规则：10次/分钟
	rl.SetRule(context.Background(), Rule{
		Name: "loose", Key: "ip", Path: "/api/.*",
		Total: 10, Duration: "1m",
	})
	// 严格规则：2次/分钟
	rl.SetRule(context.Background(), Rule{
		Name: "strict", Key: "ip", Path: "/api/.*",
		Total: 2, Duration: "1m",
	})

	ctx := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")

	// 前 2 次放行
	for i := 0; i < 2; i++ {
		r := rl.Allow(ctx)
		if !r.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 第 3 次应被严格规则拦截
	r := rl.Allow(ctx)
	if r.Allowed {
		t.Fatal("3rd request should be blocked by strict rule")
	}
	if r.Total != 2 {
		t.Fatalf("expected total=2 (strict rule), got=%d", r.Total)
	}
}

// TestPathRegex 测试正则路径匹配
func TestPathRegex(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "v1", Key: "ip", Path: "/v1/.*",
		Total: 1, Duration: "1m",
	})

	// 匹配的路径
	ctx := buildCtx("/v1/users", "GET", meta.MetaUserIp, "1.2.3.4")
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("should match /v1/.*")
	}

	// 不匹配的路径，应直接放行（无规则匹配）
	ctx2 := buildCtx("/v2/users", "GET", meta.MetaUserIp, "1.2.3.4")
	r2 := rl.Allow(ctx2)
	if !r2.Allowed {
		t.Fatal("/v2/users should not match /v1/.* rule")
	}
}

// TestExactPathMatch 测试精确路径匹配（无正则字符）
func TestExactPathMatch(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "exact", Key: "ip", Path: "/api/login",
		Total: 1, Duration: "1m",
	})

	// 精确匹配
	ctx := buildCtx("/api/login", "POST", meta.MetaUserIp, "1.2.3.4")
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("should match exact path")
	}

	// 不匹配
	ctx2 := buildCtx("/api/login/extra", "POST", meta.MetaUserIp, "1.2.3.4")
	r2 := rl.Allow(ctx2)
	if !r2.Allowed || r2.Remaining != -1 {
		t.Fatal("should not match different exact path")
	}
}

// TestMethodFilter 测试指定方法过滤
func TestMethodFilter(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "post_only", Key: "ip", Path: "/api/.*",
		Methods: []string{"POST"}, Total: 1, Duration: "1m",
	})

	// POST 应被限速
	ctx := buildCtx("/api/submit", "POST", meta.MetaUserIp, "1.2.3.4")
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("first POST should be allowed")
	}
	r2 := rl.Allow(ctx)
	if r2.Allowed {
		t.Fatal("second POST should be blocked")
	}

	// GET 不受限速影响
	ctxGet := buildCtx("/api/submit", "GET", meta.MetaUserIp, "1.2.3.4")
	r3 := rl.Allow(ctxGet)
	if !r3.Allowed {
		t.Fatal("GET should not be affected by POST-only rule")
	}
}

// TestEmptyMethodsMatchAll 测试 methods 为空时匹配所有方法
func TestEmptyMethodsMatchAll(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "all_methods", Key: "ip", Path: "/api/.*",
		Total: 2, Duration: "1m",
	})

	ctx1 := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")
	ctx2 := buildCtx("/api/test", "POST", meta.MetaUserIp, "1.2.3.4")

	r1 := rl.Allow(ctx1)
	r2 := rl.Allow(ctx2)

	if !r1.Allowed || !r2.Allowed {
		t.Fatal("both GET and POST should be allowed (empty methods = match all)")
	}
}

// TestMetaKeyExtract 测试不同 meta key 的正确提取
func TestMetaKeyExtract(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "by_device", Key: "device_id", Path: "/api/.*",
		Total: 1, Duration: "1m",
	})

	ctx := buildCtx("/api/test", "GET", meta.MetaDeviceId, "device-abc")
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("first request by device should be allowed")
	}

	// 同 device_id 第二次应被拦截
	r2 := rl.Allow(ctx)
	if r2.Allowed {
		t.Fatal("second request by same device should be blocked")
	}

	// 不同 device_id 不受影响
	ctx2 := buildCtx("/api/test", "GET", meta.MetaDeviceId, "device-xyz")
	r3 := rl.Allow(ctx2)
	if !r3.Allowed {
		t.Fatal("different device should be allowed")
	}
}

// TestSetDeleteRule 测试动态增删规则后生效
func TestSetDeleteRule(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	ctx := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")

	// 无规则，直接放行
	r := rl.Allow(ctx)
	if !r.Allowed || r.Remaining != -1 {
		t.Fatal("should be allowed when no rules")
	}

	// 添加规则
	rl.SetRule(context.Background(), Rule{
		Name: "temp", Key: "ip", Path: "/api/test",
		Total: 1, Duration: "1m",
	})

	r2 := rl.Allow(ctx)
	if !r2.Allowed {
		t.Fatal("first request should be allowed after adding rule")
	}
	r3 := rl.Allow(ctx)
	if r3.Allowed {
		t.Fatal("second request should be blocked")
	}

	// 删除规则后放行
	rl.DeleteRule(context.Background(), "temp")
	r4 := rl.Allow(ctx)
	if !r4.Allowed {
		t.Fatal("should be allowed after deleting rule")
	}
}

// TestEmptyKeySkip 测试 meta 中无对应 key 时跳过该规则且放行
func TestEmptyKeySkip(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "token_limit", Key: "token", Path: "/api/.*",
		Total: 1, Duration: "1m",
	})

	// ctx 中没有 token，应跳过规则直接放行
	ctx := buildCtx("/api/test", "GET")
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("should be allowed when meta key not present")
	}
	if r.Remaining != -1 {
		t.Fatal("remaining should be -1 when no rule matched")
	}
}

// TestDurationReset 测试 Duration 过期后计数器重置
func TestDurationReset(t *testing.T) {
	mr, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "quick", Key: "ip", Path: "/api/test",
		Total: 1, Duration: "10s",
	})

	ctx := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")

	// 第 1 次放行
	r := rl.Allow(ctx)
	if !r.Allowed {
		t.Fatal("should be allowed")
	}

	// 第 2 次被拦截
	r2 := rl.Allow(ctx)
	if r2.Allowed {
		t.Fatal("should be blocked")
	}

	// 快进 11 秒，计数器过期
	mr.FastForward(11 * time.Second)

	// 重置后应放行
	r3 := rl.Allow(ctx)
	if !r3.Allowed {
		t.Fatal("should be allowed after counter reset")
	}
}

// TestQuotaHeaders 测试 Quota 中 Total / Remaining / ResetAt 正确
func TestQuotaHeaders(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := newTestLimiter(t, rdb)

	rl.SetRule(context.Background(), Rule{
		Name: "header_test", Key: "ip", Path: "/api/test",
		Total: 5, Duration: "1m",
	})

	ctx := buildCtx("/api/test", "GET", meta.MetaUserIp, "1.2.3.4")

	r := rl.Allow(ctx)
	if r.Total != 5 {
		t.Fatalf("expected total=5, got=%d", r.Total)
	}
	if r.Remaining != 4 {
		t.Fatalf("expected remaining=4, got=%d", r.Remaining)
	}
	if r.ResetAt == 0 {
		t.Fatal("reset_at should be set")
	}
}

// TestResolveMetaKey 测试 key 映射
func TestResolveMetaKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ip", meta.MetaUserIp},
		{"device_id", meta.MetaDeviceId},
		{"token", meta.MetaToken},
		{"user_id", meta.MetaUserId},
		{"platform", meta.MetaPlatform},
		{"custom", "x-meta-custom"},
	}

	for _, tt := range tests {
		got := resolveMetaKey(tt.input)
		if got != tt.want {
			t.Errorf("resolveMetaKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestPrefixIsolation 测试不同前缀的限速器数据隔离
func TestPrefixIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl1 := newTestLimiter(t, rdb, WithPrefix("api"))
	rl2 := newTestLimiter(t, rdb, WithPrefix("admin"))

	rl1.SetRule(context.Background(), Rule{
		Name: "limit", Key: "ip", Path: "/test",
		Total: 1, Duration: "1m",
	})

	ctx := buildCtx("/test", "GET", meta.MetaUserIp, "1.2.3.4")

	// rl1 规则生效
	r1 := rl1.Allow(ctx)
	if !r1.Allowed {
		t.Fatal("rl1: first request should be allowed")
	}

	// rl2 无规则，放行
	r2 := rl2.Allow(ctx)
	if !r2.Allowed || r2.Remaining != -1 {
		t.Fatal("rl2: should have no rules and allow")
	}
}
