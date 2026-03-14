package ginlog

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Options 日志中间件配置
type Options struct {
	// SlowThreshold 慢请求阈值，超过此时间的请求会被标记为慢请求
	SlowThreshold time.Duration
}

// New 创建一个新的Gin日志中间件，使用zerolog
func New(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算耗时
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		userAgent := c.Request.UserAgent()

		if raw != "" {
			path = path + "?" + raw
		}

		// 构建日志事件
		var event *zerolog.Event
		if status >= 500 {
			event = log.Error()
		} else if status >= 400 {
			event = log.Warn()
		} else if opts.SlowThreshold > 0 && latency > opts.SlowThreshold {
			event = log.Warn()
		} else {
			if !viper.GetBool("log.traced") {
				return
			}
			event = log.Debug()
		}

		event.
			Str("method", method).
			Str("path", path).
			Int("status", status).
			Str("client_ip", clientIP).
			Str("user_agent", userAgent).
			Dur("latency", latency).
			Int("body_size", c.Writer.Size())

		// 如果有错误，记录错误信息
		if len(c.Errors) > 0 {
			event.Str("errors", c.Errors.ByType(gin.ErrorTypePrivate).String())
		}

		// 标记慢请求
		if opts.SlowThreshold > 0 && latency > opts.SlowThreshold {
			event.Bool("slow", true)
		}

		event.Msg("http request")
	}
}
