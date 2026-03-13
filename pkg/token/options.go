package token

import "time"

// options 是 TokenManager 的配置项
type options struct {
	prefix     string
	accessTtl  time.Duration
	refreshTtl time.Duration
	unique     bool
	genToken   func() string
}

var defaultOptions = options{
	prefix:     "token",
	accessTtl:  2 * time.Hour,
	refreshTtl: 7 * 24 * time.Hour,
	genToken:   generateToken,
}

// Option 是 TokenManager 配置函数
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认 "token"
//
//	token.NewTokenManager(rdb, token.WithPrefix("admin"))
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithAccessTtl 设置 Access Token 过期时间，默认 2 小时
func WithAccessTtl(d time.Duration) Option {
	return func(o *options) { o.accessTtl = d }
}

// WithRefreshTtl 设置 Refresh Token 过期时间，默认 7 天
func WithRefreshTtl(d time.Duration) Option {
	return func(o *options) { o.refreshTtl = d }
}

// WithUnique 开启平台唯一性约束，同 user_id + platform 仅保留一个有效 Token
func WithUnique() Option {
	return func(o *options) { o.unique = true }
}

// WithTokenGenerator 自定义 Token 生成函数
func WithTokenGenerator(fn func() string) Option {
	return func(o *options) { o.genToken = fn }
}
