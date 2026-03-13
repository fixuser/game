package token

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis 创建 miniredis 实例和对应的 Redis 客户端
func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// TestCreateRaw 测试基本的 Token 创建和验证
func TestCreateRaw(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	tv, err := tm.CreateRaw(context.Background(), 100, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if tv.UserId != 100 {
		t.Fatalf("expected user_id=100, got=%d", tv.UserId)
	}
	if tv.UserType != "user" {
		t.Fatalf("expected user_type=user, got=%s", tv.UserType)
	}
	if tv.Platform != "ios" {
		t.Fatalf("expected platform=ios, got=%s", tv.Platform)
	}
	if tv.AccessToken == "" || tv.RefreshToken == "" {
		t.Fatal("access_token or refresh_token is empty")
	}

	// Verify 能拿到正确的 TokenValue
	got, err := tm.Verify(context.Background(), tv.AccessToken)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if got.UserId != 100 {
		t.Fatalf("verify: expected user_id=100, got=%d", got.UserId)
	}
	if got.Platform != "ios" {
		t.Fatalf("verify: expected platform=ios, got=%s", got.Platform)
	}
}

// TestCreateFromValue 测试通过 TokenValue 实例创建 Token
func TestCreateFromValue(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	input := &TokenValue{
		UserId:   1001,
		UserType: "guest",
		Platform: "web",
		Extras:   []byte(`{"foo":"bar"}`),
	}

	tv, err := tm.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if tv.UserId != input.UserId || tv.UserType != input.UserType || tv.Platform != input.Platform {
		t.Fatal("basic fields mismatch")
	}
	if tv.GetExtra("foo").String() != "bar" {
		t.Fatal("extras mismatch")
	}
	if tv.AccessToken == "" || tv.RefreshToken == "" {
		t.Fatal("tokens not generated")
	}

	// Verify
	got, err := tm.Verify(context.Background(), tv.AccessToken)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if got.UserId != 1001 {
		t.Fatal("user identity mismatch after verify")
	}
}

// TestRefresh 测试刷新后旧 Token 失效、新 Token 可用
func TestRefresh(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	tv, err := tm.CreateRaw(context.Background(), 200, "user", "android", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	oldAccess := tv.AccessToken
	oldRefresh := tv.RefreshToken

	newTV, err := tm.Refresh(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	// 新 token 应不同于旧 token
	if newTV.AccessToken == oldAccess {
		t.Fatal("new access_token should differ from old")
	}
	if newTV.RefreshToken == oldRefresh {
		t.Fatal("new refresh_token should differ from old")
	}
	// 用户信息应保持一致
	if newTV.UserId != 200 {
		t.Fatalf("expected user_id=200, got=%d", newTV.UserId)
	}

	// 旧 access token 应失效
	if _, err := tm.Verify(context.Background(), oldAccess); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound for old token, got=%v", err)
	}

	// 新 access token 应可用
	got, err := tm.Verify(context.Background(), newTV.AccessToken)
	if err != nil {
		t.Fatalf("verify new token failed: %v", err)
	}
	if got.UserId != 200 {
		t.Fatalf("expected user_id=200, got=%d", got.UserId)
	}
}

// TestRevoke 测试注销后 Token 不可用
func TestRevoke(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	tv, err := tm.CreateRaw(context.Background(), 300, "admin", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := tm.Revoke(context.Background(), tv.AccessToken); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}

	// 验证已失效
	if _, err := tm.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound after revoke, got=%v", err)
	}

	// Refresh 也应失败
	if _, err := tm.Refresh(context.Background(), tv.RefreshToken); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected ErrRefreshTokenNotFound after revoke, got=%v", err)
	}
}

// TestRevokeAlreadyExpired 测试注销一个不存在的 token 不报错
func TestRevokeAlreadyExpired(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	if err := tm.Revoke(context.Background(), "nonexistent-token"); err != nil {
		t.Fatalf("revoke of nonexistent token should not error, got=%v", err)
	}
}

// TestUniqueCreateRaw 测试开启 unique 后，同 user+platform 重复创建会踢掉旧 token
func TestUniqueCreateRaw(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithUnique())

	tv1, err := tm.CreateRaw(context.Background(), 400, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create tv1 failed: %v", err)
	}

	tv2, err := tm.CreateRaw(context.Background(), 400, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create tv2 failed: %v", err)
	}

	// 旧 token 应失效
	if _, err := tm.Verify(context.Background(), tv1.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected old token revoked, got=%v", err)
	}

	// 新 token 应可用
	got, err := tm.Verify(context.Background(), tv2.AccessToken)
	if err != nil {
		t.Fatalf("verify tv2 failed: %v", err)
	}
	if got.UserId != 400 {
		t.Fatalf("expected user_id=400, got=%d", got.UserId)
	}
}

// TestUniqueDisabled 测试未开启 unique 时同 user+platform 可并存多个 token
func TestUniqueDisabled(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb) // 默认不开启 unique

	tv1, err := tm.CreateRaw(context.Background(), 500, "user", "web", nil)
	if err != nil {
		t.Fatalf("create tv1 failed: %v", err)
	}

	tv2, err := tm.CreateRaw(context.Background(), 500, "user", "web", nil)
	if err != nil {
		t.Fatalf("create tv2 failed: %v", err)
	}

	// 两个 token 都应有效
	if _, err := tm.Verify(context.Background(), tv1.AccessToken); err != nil {
		t.Fatalf("tv1 should still be valid, got=%v", err)
	}
	if _, err := tm.Verify(context.Background(), tv2.AccessToken); err != nil {
		t.Fatalf("tv2 should still be valid, got=%v", err)
	}
}

// TestPrefixIsolation 测试不同前缀的 TokenManager 数据互相隔离
func TestPrefixIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	userTM := NewTokenManager(rdb, WithPrefix("user"))
	adminTM := NewTokenManager(rdb, WithPrefix("admin"))

	tv, err := userTM.CreateRaw(context.Background(), 600, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 用 admin 前缀的 manager 验证同一个 token 应失败
	if _, err := adminTM.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected token not found across prefix, got=%v", err)
	}

	// 用 user 前缀可以正常验证
	if _, err := userTM.Verify(context.Background(), tv.AccessToken); err != nil {
		t.Fatalf("verify with correct prefix failed: %v", err)
	}
}

// TestExtras 测试 SetExtra / GetExtra 读写正确性
func TestExtras(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	extras := []byte(`{}`)
	tv, err := tm.CreateRaw(context.Background(), 700, "user", "web", extras)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	tv.SetExtra("role", "admin")
	tv.SetExtra("level", 5)

	if tv.GetExtra("role").String() != "admin" {
		t.Fatalf("expected role=admin, got=%s", tv.GetExtra("role").String())
	}
	if tv.GetExtra("level").Int() != 5 {
		t.Fatalf("expected level=5, got=%d", tv.GetExtra("level").Int())
	}

	// 未设置的 key 应返回空
	if tv.GetExtra("nonexistent").String() != "" {
		t.Fatalf("expected empty string for nonexistent key, got=%s", tv.GetExtra("nonexistent").String())
	}
}

// TestAccessTokenExpiry 测试 Access Token 过期后 Verify 返回错误
func TestAccessTokenExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithAccessTtl(10*time.Second))

	tv, err := tm.CreateRaw(context.Background(), 800, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 快进让 access token 过期
	mr.FastForward(11 * time.Second)

	if _, err := tm.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound after expiry, got=%v", err)
	}
}

// TestRefreshTokenExpiry 测试 Refresh Token 过期后无法刷新
func TestRefreshTokenExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithAccessTtl(10*time.Second), WithRefreshTtl(30*time.Second))

	tv, err := tm.CreateRaw(context.Background(), 900, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 快进让 refresh token 过期
	mr.FastForward(31 * time.Second)

	if _, err := tm.Refresh(context.Background(), tv.RefreshToken); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected ErrRefreshTokenNotFound after expiry, got=%v", err)
	}
}

// TestCustomTokenGenerator 测试自定义 Token 生成函数生效
func TestCustomTokenGenerator(t *testing.T) {
	_, rdb := newTestRedis(t)
	counter := 0
	gen := func() string {
		counter++
		if counter%2 == 1 {
			return "custom-access"
		}
		return "custom-refresh"
	}

	tm := NewTokenManager(rdb, WithTokenGenerator(gen))

	tv, err := tm.CreateRaw(context.Background(), 1000, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if tv.AccessToken != "custom-access" {
		t.Fatalf("expected access_token=custom-access, got=%s", tv.AccessToken)
	}
	if tv.RefreshToken != "custom-refresh" {
		t.Fatalf("expected refresh_token=custom-refresh, got=%s", tv.RefreshToken)
	}
}

// TestBinaryMarshalUnmarshal 测试 TokenValue 的 BinaryMarshaler / BinaryUnmarshaler 实现
func TestBinaryMarshalUnmarshal(t *testing.T) {
	original := TokenValue{
		UserId:       42,
		UserType:     "admin",
		AccessToken:  "abc123",
		RefreshToken: "def456",
		Platform:     "web",
		CreatedAt:    1000,
		AccessTtl:    7200,
		RefreshTtl:   604800,
		Extras:       []byte(`{"role":"super"}`),
	}

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded TokenValue
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.UserId != original.UserId {
		t.Fatalf("user_id mismatch: %d vs %d", decoded.UserId, original.UserId)
	}
	if decoded.AccessToken != original.AccessToken {
		t.Fatalf("access_token mismatch: %s vs %s", decoded.AccessToken, original.AccessToken)
	}
	if decoded.GetExtra("role").String() != "super" {
		t.Fatalf("extras mismatch: got=%s", decoded.GetExtra("role").String())
	}
}
