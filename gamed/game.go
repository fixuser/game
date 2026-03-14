package gamed

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/game/game/apis/userpb"
	"github.com/game/game/pkg/boot"
	"github.com/game/game/pkg/meta"
	"github.com/game/game/pkg/msgcode"
	"github.com/game/game/pkg/ratelimit"
	"github.com/game/game/pkg/red/redlb"
	"github.com/game/game/pkg/token"
	"github.com/game/game/services/user"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	traceSdk "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type gameServer struct {
	mu             sync.RWMutex
	wg             sync.WaitGroup
	opts           *option
	db             *bun.DB
	rdbs           *boot.RedisStore
	registry       *redlb.Registry
	lease          *redlb.Lease
	tracerProvider *traceSdk.TracerProvider
	rateLimiter    *ratelimit.RateLimiter
	manToken       *token.ManToken
	userToken      *token.UserToken
	msgCode        *msgcode.Manager
	keyMutex       *boot.KeyMutex[string]
	httpServer     *http.Server
	tcpServer      net.Listener
	startAt        time.Time
	isClosed       atomic.Bool
	exitChan       chan struct{}
}

func New(opts *option) *gameServer {
	sim := &gameServer{
		opts:     opts,
		startAt:  time.Now(),
		rdbs:     boot.NewRedisStore(),
		keyMutex: boot.NewKeyMutex[string](),
		exitChan: make(chan struct{}),
	}
	return sim
}

// Main starts the server.
func (s *gameServer) Main() {
	err := s.init()
	if err != nil {
		log.Fatal().Err(err).Msg("init error")
	}

	// 简单注入全局变量
	s.msgCode = msgcode.NewManager(s.rdbs.MustGet("token", 0))
	boot.Set(s.rdbs, s.db)
	boot.Set(s.keyMutex)
	boot.Set(s.manToken, s.userToken, s.msgCode)

	// 加载模块
	err = boot.Load(boot.GetBoot().Context(context.Background()))
	if err != nil {
		log.Fatal().Err(err).Msg("load error")
	}

	// 初始化grpc-gateway
	backoff.DefaultConfig.MaxDelay = time.Second * 5

	ctx := context.Background()
	errorOption := runtime.WithErrorHandler(gatewayErrorHandler)
	metadataOption := runtime.WithMetadata(meta.Annotator)
	tcpAddr := viper.GetString("grpc.addr")
	marshalOption := runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions:   jsonpbMarshaler,
		UnmarshalOptions: jsonpbUnmarshaler,
	})
	gwmux := runtime.NewServeMux(marshalOption, errorOption, metadataOption)
	gwmux.HandlePath("POST", "/v3/man/file/upload", s.updateFileHandle)

	// 初始化grpc服务
	gsrv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(s.unaryServerInterceptor),
	)
	reflection.Register(gsrv)
	// manpb.RegisterManServiceServer(gsrv, man.ManProvider)
	userpb.RegisterUserServerServer(gsrv, user.UserService)

	// 启动grpc服务
	if viper.GetBool("grpc.enabled") {
		s.tcpServer, err = net.Listen("tcp", tcpAddr)
		if err != nil {
			log.Fatal().Err(err).Msg("listen tcp error")
		}
		go func() {
			log.Info().Str("tcp_addr", tcpAddr).Msg("start grpc server")
			err := gsrv.Serve(s.tcpServer)
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Fatal().Err(err).Send()
			}
		}()
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1.0 * time.Second,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   5 * time.Second,
			},
			MinConnectTimeout: time.Second * 5,
		}),
	}
	// manpb.RegisterManServiceHandlerFromEndpoint(ctx, gwmux, tcpAddr, opts)
	userpb.RegisterUserServerHandlerFromEndpoint(ctx, gwmux, tcpAddr, opts)

	// 启动http服务
	if viper.GetBool("http.enabled") {
		httpAddr := viper.GetString("http.addr")

		go func() {
			log.Info().Str("http_addr", httpAddr).Msg("start http server")
			s.httpServer = &http.Server{Addr: httpAddr, Handler: initHttpHandler(gwmux)}
			err := s.httpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Err(err).Send()
			}
		}()
	}

	// 注册服务
	if s.registry != nil {
		s.lease, _, err = s.registry.Register(context.Background(), "")
		if err != nil {
			log.Fatal().Err(err).Msg("register service error")
		}
	}
}

// Exit closes the server.
func (s *gameServer) Exit() {
	log.Info().Float64("running", time.Since(s.startAt).Seconds()).Msg("server exiting")

	start := time.Now()

	// 关闭服务，通知其他监听函数
	close(s.exitChan)

	// 关闭API服务，此时API返回维护中错误
	s.isClosed.Store(true)

	// 注销服务
	if s.registry != nil && s.lease != nil {
		s.lease.Stop(context.Background())
	}

	// 退出所有模块
	boot.Unload(boot.GetBoot().Context(context.Background()))

	// 等待其他常驻内存函数结束
	s.wg.Wait()

	// 链路追踪关闭
	if s.tracerProvider != nil {
		go s.tracerProvider.Shutdown(context.Background())
	}

	// 关闭服务
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.tcpServer != nil {
		s.tcpServer.Close()
	}

	spent := time.Since(start).Seconds() * 1e3
	log.Info().Float64("spent", spent).Msg("server exited")

	// 在等待一会儿，确保所有的请求都处理完毕
	time.Sleep(time.Millisecond * 200)

	// 关闭数据库连接
	for _, db := range s.rdbs.Items() {
		db.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}
