package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var (
	// ErrTokenNotFound 表示 Token 不存在或已过期
	ErrTokenNotFound = errors.New("token: not found")
	// ErrTokenValueNil 表示 TokenValue 为 nil
	ErrTokenValueNil = errors.New("token: token value is nil")
	// ErrRefreshTokenNotFound 表示 Refresh Token 不存在或已过期
	ErrRefreshTokenNotFound = errors.New("token: refresh token not found")
)

// TokenManager 是 Redis 驱动的 Token 管理器
type TokenManager struct {
	rdb redis.UniversalClient
	opt options
}

// NewTokenManager 创建一个 Token 管理器
//
//	tm := token.NewTokenManager(rdb, token.WithPrefix("user"), token.WithUnique())
func NewTokenManager(rdb redis.UniversalClient, opts ...Option) *TokenManager {
	o := defaultOptions
	for _, fn := range opts {
		fn(&o)
	}
	return &TokenManager{rdb: rdb, opt: o}
}

// Create 从 TokenValue 实例创建一个新的 Token（仅使用其 UserId, UserType, Platform, Extras 字段）
// 最终逻辑由 CreateRaw 实现
func (tm *TokenManager) Create(ctx context.Context, tv *TokenValue) (*TokenValue, error) {
	if tv == nil {
		return nil, ErrTokenValueNil
	}
	return tm.CreateRaw(ctx, tv.UserId, tv.UserType, tv.Platform, tv.Extras)
}

// CreateRaw 直接使用原始参数创建一对 Access Token 和 Refresh Token 并写入 Redis
// 如果开启了 unique 模式，会先删除同 user_id + platform 的旧 Token
func (tm *TokenManager) CreateRaw(ctx context.Context, userId int64, userType, platform string, extras []byte) (*TokenValue, error) {
	now := time.Now().Unix()
	tv := &TokenValue{
		UserId:       userId,
		UserType:     userType,
		AccessToken:  tm.opt.genToken(),
		RefreshToken: tm.opt.genToken(),
		Platform:     platform,
		CreatedAt:    now,
		AccessTtl:    int64(tm.opt.accessTtl.Seconds()),
		RefreshTtl:   int64(tm.opt.refreshTtl.Seconds()),
		Extras:       extras,
	}

	pipe := tm.rdb.Pipeline()

	// 如果启用唯一约束，先查旧 token 并清理
	if tm.opt.unique {
		ukey := tm.uniqueKey(userId, platform)
		oldAccess, err := tm.rdb.Get(ctx, ukey).Result()
		if err == nil && oldAccess != "" {
			// 读取旧 token 的 refresh_token 以便一并清理
			var oldTV TokenValue
			if err := tm.rdb.Get(ctx, tm.tokenKey(oldAccess)).Scan(&oldTV); err == nil {
				pipe.Del(ctx, tm.refreshKey(oldTV.RefreshToken))
			}
			pipe.Del(ctx, tm.tokenKey(oldAccess))
			log.Ctx(ctx).Info().Int64("user_id", userId).Str("platform", platform).Msg("old token revoked due to unique constraint")
		}
		pipe.Set(ctx, ukey, tv.AccessToken, tm.opt.refreshTtl)
	}

	pipe.Set(ctx, tm.tokenKey(tv.AccessToken), tv, tm.opt.accessTtl)
	pipe.Set(ctx, tm.refreshKey(tv.RefreshToken), tv.AccessToken, tm.opt.refreshTtl)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("token: create failed: %w", err)
	}

	log.Ctx(ctx).Info().Int64("user_id", userId).Str("platform", platform).Msg("token created")
	return tv, nil
}

// Verify 验证 Access Token 是否有效，有效则返回对应的 TokenValue
func (tm *TokenManager) Verify(ctx context.Context, accessToken string) (*TokenValue, error) {
	var tv TokenValue
	if err := tm.rdb.Get(ctx, tm.tokenKey(accessToken)).Scan(&tv); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token: verify failed: %w", err)
	}
	return &tv, nil
}

// Refresh 使用 Refresh Token 生成新的 Access Token 和 Refresh Token
// 旧的 Access Token 和 Refresh Token 会立即失效
func (tm *TokenManager) Refresh(ctx context.Context, refreshToken string) (*TokenValue, error) {
	// 1. 通过 refresh token 找到旧的 access token
	oldAccess, err := tm.rdb.Get(ctx, tm.refreshKey(refreshToken)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("token: refresh lookup failed: %w", err)
	}

	// 2. 读取旧的 TokenValue
	var oldTV TokenValue
	if err := tm.rdb.Get(ctx, tm.tokenKey(oldAccess)).Scan(&oldTV); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token: refresh read failed: %w", err)
	}

	// 3. 生成新 Token
	newTV := &TokenValue{
		UserId:       oldTV.UserId,
		UserType:     oldTV.UserType,
		AccessToken:  tm.opt.genToken(),
		RefreshToken: tm.opt.genToken(),
		Platform:     oldTV.Platform,
		CreatedAt:    time.Now().Unix(),
		AccessTtl:    oldTV.AccessTtl,
		RefreshTtl:   oldTV.RefreshTtl,
		Extras:       oldTV.Extras,
	}

	// 4. Pipeline：删旧写新
	pipe := tm.rdb.Pipeline()
	pipe.Del(ctx, tm.tokenKey(oldAccess))
	pipe.Del(ctx, tm.refreshKey(refreshToken))
	pipe.Set(ctx, tm.tokenKey(newTV.AccessToken), newTV, tm.opt.accessTtl)
	pipe.Set(ctx, tm.refreshKey(newTV.RefreshToken), newTV.AccessToken, tm.opt.refreshTtl)

	if tm.opt.unique {
		pipe.Set(ctx, tm.uniqueKey(newTV.UserId, newTV.Platform), newTV.AccessToken, tm.opt.refreshTtl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("token: refresh failed: %w", err)
	}

	log.Ctx(ctx).Info().Int64("user_id", newTV.UserId).Str("platform", newTV.Platform).Msg("token refreshed")
	return newTV, nil
}

// Revoke 主动注销 Access Token，同时清理对应的 Refresh Token 和 unique key
func (tm *TokenManager) Revoke(ctx context.Context, accessToken string) error {
	var tv TokenValue
	if err := tm.rdb.Get(ctx, tm.tokenKey(accessToken)).Scan(&tv); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil // 已过期或不存在，视为注销成功
		}
		return fmt.Errorf("token: revoke read failed: %w", err)
	}

	pipe := tm.rdb.Pipeline()
	pipe.Del(ctx, tm.tokenKey(accessToken))
	pipe.Del(ctx, tm.refreshKey(tv.RefreshToken))

	if tm.opt.unique {
		pipe.Del(ctx, tm.uniqueKey(tv.UserId, tv.Platform))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("token: revoke failed: %w", err)
	}

	log.Ctx(ctx).Info().Int64("user_id", tv.UserId).Str("platform", tv.Platform).Msg("token revoked")
	return nil
}

// tokenKey 生成 access token 的 Redis key
func (tm *TokenManager) tokenKey(accessToken string) string {
	return tm.opt.prefix + ":token:" + accessToken
}

// refreshKey 生成 refresh token 的 Redis key
func (tm *TokenManager) refreshKey(refreshToken string) string {
	return tm.opt.prefix + ":refresh:" + refreshToken
}

// uniqueKey 生成唯一性约束的 Redis key
func (tm *TokenManager) uniqueKey(userId int64, platform string) string {
	return fmt.Sprintf("%s:unique:%d:%s", tm.opt.prefix, userId, platform)
}

// generateToken 生成 32 字符的随机 hex 字符串
func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
