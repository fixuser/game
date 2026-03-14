package redlb

import (
	"time"

	"github.com/game/game/pkg/boot"
	"github.com/spf13/viper"
)

// options 是 redlb 的全局配置。
type options struct {
	prefix            string
	ttl               time.Duration
	heartbeatInterval time.Duration
	bootInfoReader    func() boot.Info
	addrReader        func(key string) string
}

var defaultOptions = options{
	prefix:            "redlb:service",
	ttl:               12 * time.Second,
	heartbeatInterval: 4 * time.Second,
	bootInfoReader:    boot.Read,
	addrReader:        viper.GetString,
}

// Option 用于配置 Registry。
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认值为 "redlb:service"。
func WithPrefix(prefix string) Option {
	return func(o *options) {
		if prefix != "" {
			o.prefix = prefix
		}
	}
}

// WithTtl 设置注册信息在 Redis 中的过期时间。
func WithTtl(ttl time.Duration) Option {
	return func(o *options) {
		if ttl > 0 {
			o.ttl = ttl
		}
	}
}

// WithHeartbeatInterval 设置心跳续期间隔。
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(o *options) {
		if interval > 0 {
			o.heartbeatInterval = interval
		}
	}
}

// WithBootInfoReader 注入 boot 信息读取函数，便于测试或自定义来源。
func WithBootInfoReader(fn func() boot.Info) Option {
	return func(o *options) {
		if fn != nil {
			o.bootInfoReader = fn
		}
	}
}

// WithAddrReader 注入地址读取函数，默认从 viper 读取 grpc.addr 和 http.addr。
func WithAddrReader(fn func(key string) string) Option {
	return func(o *options) {
		if fn != nil {
			o.addrReader = fn
		}
	}
}

// registerOptions 是单次注册时的覆盖配置。
type registerOptions struct {
	instanceId string
	ip         string
	hostname   string
	grpcAddr   string
	httpAddr   string
}

// RegisterOption 用于覆盖默认注册行为。
type RegisterOption func(*registerOptions)

// WithRegisterInstanceId 指定注册实例 ID。
func WithRegisterInstanceId(id string) RegisterOption {
	return func(o *registerOptions) {
		o.instanceId = id
	}
}

// WithRegisterIp 指定注册 IP，覆盖 boot.Read() 结果。
func WithRegisterIp(ip string) RegisterOption {
	return func(o *registerOptions) {
		o.ip = ip
	}
}

// WithRegisterHostname 指定主机名，覆盖 boot.Read() 结果。
func WithRegisterHostname(hostname string) RegisterOption {
	return func(o *registerOptions) {
		o.hostname = hostname
	}
}

// WithRegisterGrpcAddr 指定 gRPC 地址，覆盖 grpc.addr。
func WithRegisterGrpcAddr(addr string) RegisterOption {
	return func(o *registerOptions) {
		o.grpcAddr = addr
	}
}

// WithRegisterHttpAddr 指定 HTTP 地址，覆盖 http.addr。
func WithRegisterHttpAddr(addr string) RegisterOption {
	return func(o *registerOptions) {
		o.httpAddr = addr
	}
}
