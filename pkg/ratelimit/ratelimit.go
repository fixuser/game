package ratelimit

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/game/game/pkg/meta"
)

// keyMapping 将简短的 key 名称映射到 meta 中的完整 key
// 配置时只需写 "ip"、"device_id"、"token"，内部自动转换
var keyMapping = map[string]string{
	"ip":        meta.MetaUserIp,
	"device_id": meta.MetaDeviceId,
	"token":     meta.MetaToken,
	"user_id":   meta.MetaUserId,
	"platform":  meta.MetaPlatform,
}

// resolveMetaKey 将短名称映射为 meta key，未找到则加上 x-meta- 前缀
func resolveMetaKey(key string) string {
	if mk, ok := keyMapping[key]; ok {
		return mk
	}
	return "x-meta-" + key
}

// Rule 是限速规则的配置结构，存储在 Redis Hash 中
type Rule struct {
	Name     string   `json:"name"`     // 规则名称（Hash field key，同名覆盖）
	Key      string   `json:"key"`      // 限速标识，如 "ip"、"device_id"、"token"
	Path     string   `json:"path"`     // 路径匹配，支持精确匹配或正则
	Methods  []string `json:"methods"`  // HTTP 方法过滤，为空匹配所有
	Total    int64    `json:"total"`    // 时间窗口内允许的总请求数
	Duration string   `json:"duration"` // 时间窗口，如 "1m"、"1h"、"1h30m"
}

// compiledRule 是 Rule 编译后的内部结构，包含解析后的正则和 Duration
type compiledRule struct {
	Rule
	metaKey  string         // 解析后的完整 meta key
	re       *regexp.Regexp // 编译后的正则（如果不是精确匹配）
	exact    bool           // 是否是精确路径匹配（无正则特殊字符）
	duration time.Duration  // 解析后的时间窗口
	methods  map[string]struct{}
}

// matchPath 检查请求路径是否匹配该规则，精确匹配优先
func (cr *compiledRule) matchPath(path string) bool {
	if cr.exact {
		return path == cr.Path
	}
	return cr.re.MatchString(path)
}

// matchMethod 检查请求方法是否匹配该规则，methods 为空时匹配所有
func (cr *compiledRule) matchMethod(method string) bool {
	if len(cr.methods) == 0 {
		return true
	}
	_, ok := cr.methods[method]
	return ok
}

// compileRule 将 Rule 编译为 compiledRule
func compileRule(r Rule) (*compiledRule, error) {
	dur, err := time.ParseDuration(r.Duration)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: invalid duration %q in rule %q: %w", r.Duration, r.Name, err)
	}

	cr := &compiledRule{
		Rule:     r,
		metaKey:  resolveMetaKey(r.Key),
		duration: dur,
		methods:  make(map[string]struct{}, len(r.Methods)),
	}

	for _, m := range r.Methods {
		cr.methods[m] = struct{}{}
	}

	// 检查路径是否包含正则特殊字符，如果没有则走精确匹配
	if isLiteralPath(r.Path) {
		cr.exact = true
	} else {
		re, err := regexp.Compile("^" + r.Path + "$")
		if err != nil {
			return nil, fmt.Errorf("ratelimit: invalid path regex %q in rule %q: %w", r.Path, r.Name, err)
		}
		cr.re = re
	}

	return cr, nil
}

// isLiteralPath 判断路径是否为纯字面量（不含正则特殊字符）
func isLiteralPath(path string) bool {
	for _, c := range path {
		switch c {
		case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			return false
		}
	}
	return true
}

// Quota 是限速检查的返回值，包含最严格规则的剩余信息
type Quota struct {
	Allowed   bool  // 是否放行
	Remaining int64 // 最严格规则的剩余次数
	Total     int64 // 最严格规则的总限额
	ResetAt   int64 // 最严格规则的重置时间（Unix 秒）
}

// luaIncrExpire 是原子性递增计数器并设置过期时间的 Lua 脚本
// KEYS[1] = counter key, ARGV[1] = duration seconds
const luaIncrExpire = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return current
`

// options 是 RateLimiter 的配置项
type options struct {
	prefix       string
	reloadPeriod time.Duration
}

var defaultOptions = options{
	prefix:       "ratelimit",
	reloadPeriod: 30 * time.Second,
}

// Option 是 RateLimiter 配置函数
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认 "ratelimit"
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithReloadPeriod 设置定时刷新规则的时间间隔，默认 30 秒
func WithReloadPeriod(d time.Duration) Option {
	return func(o *options) { o.reloadPeriod = d }
}

// RateLimiter 是 Redis 驱动的限速器，支持多规则匹配
type RateLimiter struct {
	rdb    redis.UniversalClient
	opt    options
	rules  []compiledRule
	mu     sync.RWMutex
	cancel context.CancelFunc
}

// NewRateLimiter 创建限速器，自动从 Redis 加载规则并启动定时刷新
//
//	rl := ratelimit.NewRateLimiter(ctx, rdb, ratelimit.WithPrefix("api"))
func NewRateLimiter(ctx context.Context, rdb redis.UniversalClient, opts ...Option) *RateLimiter {
	o := defaultOptions
	for _, fn := range opts {
		fn(&o)
	}

	rl := &RateLimiter{
		rdb: rdb,
		opt: o,
	}

	// 首次加载规则
	if err := rl.LoadRules(ctx); err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("ratelimit: initial load rules failed")
	}

	// 启动定时刷新
	reloadCtx, cancel := context.WithCancel(ctx)
	rl.cancel = cancel
	go rl.reloadLoop(reloadCtx)

	return rl
}

// Close 停止定时刷新
func (rl *RateLimiter) Close() {
	if rl.cancel != nil {
		rl.cancel()
	}
}

// reloadLoop 定时从 Redis 加载规则
func (rl *RateLimiter) reloadLoop(ctx context.Context) {
	ticker := time.NewTicker(rl.opt.reloadPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := rl.LoadRules(ctx); err != nil {
				log.Ctx(ctx).Warn().Err(err).Msg("ratelimit: reload rules failed")
			}
		}
	}
}

// rulesKey 返回存储规则的 Redis Hash key
func (rl *RateLimiter) rulesKey() string {
	return rl.opt.prefix + ":rules"
}

// LoadRules 从 Redis Hash 加载所有规则并编译到内存
func (rl *RateLimiter) LoadRules(ctx context.Context) error {
	data, err := rl.rdb.HGetAll(ctx, rl.rulesKey()).Result()
	if err != nil {
		return fmt.Errorf("ratelimit: hgetall failed: %w", err)
	}

	rules := make([]compiledRule, 0, len(data))
	for _, raw := range data {
		var r Rule
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			log.Ctx(ctx).Warn().Str("raw", raw).Err(err).Msg("ratelimit: skip invalid rule")
			continue
		}
		cr, err := compileRule(r)
		if err != nil {
			log.Ctx(ctx).Warn().Str("rule", r.Name).Err(err).Msg("ratelimit: skip invalid rule")
			continue
		}
		rules = append(rules, *cr)
	}

	rl.mu.Lock()
	rl.rules = rules
	rl.mu.Unlock()

	log.Ctx(ctx).Debug().Int("count", len(rules)).Msg("ratelimit: rules loaded")
	return nil
}

// SetRule 写入或更新一条规则到 Redis Hash，并立即刷新内存缓存
func (rl *RateLimiter) SetRule(ctx context.Context, rule Rule) error {
	data, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("ratelimit: marshal rule failed: %w", err)
	}

	if err := rl.rdb.HSet(ctx, rl.rulesKey(), rule.Name, string(data)).Err(); err != nil {
		return fmt.Errorf("ratelimit: hset failed: %w", err)
	}

	log.Ctx(ctx).Info().Str("rule", rule.Name).Msg("ratelimit: rule set")
	return rl.LoadRules(ctx)
}

// DeleteRule 从 Redis Hash 中删除一条规则，并立即刷新内存缓存
func (rl *RateLimiter) DeleteRule(ctx context.Context, name string) error {
	if err := rl.rdb.HDel(ctx, rl.rulesKey(), name).Err(); err != nil {
		return fmt.Errorf("ratelimit: hdel failed: %w", err)
	}

	log.Ctx(ctx).Info().Str("rule", name).Msg("ratelimit: rule deleted")
	return rl.LoadRules(ctx)
}

// Allow 执行限速检查，遍历所有匹配的规则并返回最严格的结果
// 从 ctx 中自动提取 path、method 和各规则的限速标识
func (rl *RateLimiter) Allow(ctx context.Context) *Quota {
	m := meta.FromContext(ctx)
	path := m.GetString(meta.MetaRequestPath)
	method := m.GetString(meta.MetaRequestMethod)

	rl.mu.RLock()
	rules := make([]compiledRule, len(rl.rules))
	copy(rules, rl.rules)
	rl.mu.RUnlock()

	// 筛选匹配的规则并获取对应的 key value
	type matchedRule struct {
		rule     compiledRule
		keyValue string
	}

	var matched []matchedRule
	for _, cr := range rules {
		if !cr.matchPath(path) || !cr.matchMethod(method) {
			continue
		}
		keyValue := m.GetString(cr.metaKey)
		if keyValue == "" {
			continue // 标识为空，跳过该规则
		}
		matched = append(matched, matchedRule{rule: cr, keyValue: keyValue})
	}

	// 无匹配规则，直接放行
	if len(matched) == 0 {
		return &Quota{Allowed: true, Remaining: -1}
	}

	// 逐条执行 Lua 脚本（每条独立 EVAL，保证原子性）
	now := time.Now().Unix()
	quota := &Quota{Allowed: true, Remaining: -1}
	first := true

	for _, m := range matched {
		counterKey := fmt.Sprintf("%s:counter:%s:%s:%s:%s",
			rl.opt.prefix, m.keyValue, method, path, m.rule.Name)
		durSec := int64(m.rule.duration.Seconds())

		val, err := rl.rdb.Eval(ctx, luaIncrExpire, []string{counterKey}, durSec).Int64()
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("rule", m.rule.Name).Msg("ratelimit: eval counter failed")
			continue
		}

		remaining := m.rule.Total - val
		resetAt := now + int64(m.rule.duration.Seconds())

		// 取剩余最少的（最严格的）规则
		if first || remaining < quota.Remaining {
			quota.Remaining = remaining
			quota.Total = m.rule.Total
			quota.ResetAt = resetAt
			first = false
		}

		if remaining < 0 {
			quota.Allowed = false
		}
	}

	return quota
}
