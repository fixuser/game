package gamed

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/game/game/apis/basepb"
	"github.com/game/game/internal/errfmt"
	"github.com/game/game/internal/utils"
	"github.com/game/game/pkg/meta"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type validator interface {
	Validate() error
}

var (
	jsonpbMarshaler = protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: true,
		UseEnumNumbers:  false,
	}

	jsonpbUnmarshaler = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

// compressData 压缩数据
func compressData(data any, length int) string {
	body := ""
	if data != nil {
		body = cast.ToString(data)
	}
	return utils.MaskString(body, length)
}

// unaryServerInterceptor 一元拦截器
// 目前只考虑非流式的请求
func (s *gameServer) unaryServerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res any, err error) {
	// 基础数据
	start := time.Now()
	length := viper.GetInt("log.length")
	methods := strings.Split(info.FullMethod, "/")
	isMan := strings.HasPrefix(info.FullMethod, "/manpb")
	service, method := methods[1], methods[2]

	// 提取元数据
	metadata := meta.FromContext(ctx)
	deviceId := metadata.GetString(meta.MetaDeviceId)
	userIp := metadata.GetString(meta.MetaUserId)
	tokenId := metadata.GetString(meta.MetaToken)
	platform := metadata.GetString(meta.MetaPlatform)

	// 获取请求头部数据
	logger := log.With().Str("grpc_service", service).Str("grpc_request", compressData(req, length)).Str("grpc_method", method).Str("user_ip", userIp).Str("user_platform", platform).Str("device_id", deviceId).Logger()

	// 如果服务关闭，返回维护中的错误信息
	if s.isClosed.Load() {
		err = errfmt.Errorf(codes.Unavailable, basepb.Code_CODE_MAINTENANCE)
		return
	}

	// 权限验证
	defer func() {
		if e := recover(); e != nil {
			stack := debug.Stack()
			fmt.Println(string(stack))
			logger.Error().Ctx(ctx).Bytes("stack", stack).Interface("panic", e).Msg("panic error")
			err = errfmt.Errorf(codes.Internal, basepb.Code_CODE_INTERNAL_SERVER)
			return
		}
	}()

	// 限速处理
	if s.rateLimiter != nil {
		quota := s.rateLimiter.Allow(ctx)
		if quota == nil || !quota.Allowed {
			logger.Warn().Msg("rate limit exceeded")
			err = errfmt.Errorf(codes.ResourceExhausted, basepb.Code_CODE_RATE_LIMIT)
			return
		}
	}

	// 如果请求需要验证，验证请求
	if v, ok := req.(validator); ok {
		if err = v.Validate(); err != nil {
			logger.Warn().Err(err).Msg("validate request failed")
			err = errfmt.Errorf(codes.InvalidArgument, basepb.Code_CODE_ARGUMENT)
			return
		}
	}

	// 认证处理
	authDisabled := viper.GetBool("auth.disabled")
	if !authDisabled {
		ctx = metadata.Context(ctx)

		val, _ := lo.Ternary(isMan, s.manToken.Get, s.userToken.Get)(ctx, tokenId)
		if val.IsTokenValid(platform) {
			metadata.Set(meta.MetaUserId, val.UserId)
			metadata.Set(meta.MetaUserType, val.UserType)
			ctx = metadata.Context(ctx)
		} else {
			// 检查白名单, 如果是鉴权白名单中的接口，则允许通过
			authWhitelists := viper.GetStringSlice("auth.whitelists")
			requestPath := metadata.GetString(meta.MetaRequestPath)
			if !slices.Contains(authWhitelists, requestPath) {
				err = errfmt.Errorf(codes.Unauthenticated, basepb.Code_CODE_UNAUTHORIZED)
				logger.Warn().Str("token_id", tokenId).Msg("invalid token")
				return
			}
		}
	}

	// 处理上下文跟请求
	span := trace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()
	if spanCtx.IsValid() {
		// 给日志注入 trace_id 和 span_id
		ctx = log.With().Str("trace_id", spanCtx.TraceID().String()).Str("span_id", spanCtx.SpanID().String()).Logger().WithContext(ctx)
		logger = logger.With().Str("trace_id", spanCtx.TraceID().String()).Str("span_id", spanCtx.SpanID().String()).Logger()
	}

	res, err = handler(ctx, req)
	spent := time.Since(start)
	logger = logger.With().Str("grpc_response", compressData(res, length)).Float64("spent", spent.Seconds()*1e3).Logger()
	if err != nil {
		// 由于自带的错误信息不够详细，这里尝试获取更详细的错误信息
		if baseErr := errfmt.Parse(err); baseErr != nil {
			span.SetAttributes(attribute.Int("error.code", int(baseErr.Code)))
			if baseErr.Message != "" {
				span.SetAttributes(attribute.String("error.message", baseErr.Message))
			}
			logger = logger.With().Int("error_code", int(baseErr.Code)).Str("error_message", baseErr.Message).Logger()
		}
		logger.Error().Err(err).Msg("request failed")
	} else {
		logger.Info().Msg("request success")
	}
	return
}

// gatewayErrorHandler 网关错误处理
func gatewayErrorHandler(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	s, ok := status.FromError(err)
	if !ok {
		s = status.New(codes.Unknown, err.Error())
	}

	w.Header().Del("Trailer")
	contentType := marshaler.ContentType(nil)
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(runtime.HTTPStatusFromCode(s.Code()))

	details := s.Details()
	if len(details) > 0 {
		buf, merr := marshaler.Marshal(details[0])
		if merr == nil {
			_, _ = w.Write(buf)
		}
		return
	}

	log.Warn().Str("path", r.URL.Path).Str("method", r.Method).Str("error", s.Message()).Str("code", s.Code().String()).Msg("gateway error")
	code := basepb.Code_CODE_INTERNAL_SERVER
	if s.Code() == codes.InvalidArgument {
		code = basepb.Code_CODE_ARGUMENT
	} else if s.Code() == codes.NotFound || s.Code() == codes.Unimplemented {
		code = basepb.Code_CODE_NOT_FOUND
	}
	buf, merr := marshaler.Marshal(errfmt.NewError(code, s.Message()))
	if merr == nil {
		_, _ = w.Write(buf)
	}
}
