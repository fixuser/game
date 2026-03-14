package gamed

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/game/game/migrations"
	"github.com/game/game/models"
	"github.com/game/game/pkg/boot"
	"github.com/game/game/pkg/logger/bunlog"
	"github.com/game/game/pkg/logger/otellog"
	"github.com/game/game/pkg/ratelimit"
	"github.com/game/game/pkg/red/redlb"
	"github.com/game/game/pkg/token"
	"github.com/game/game/services/user"
	"github.com/go-logr/logr"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dbfixture"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	traceSdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

func (s *gameServer) init() error {
	s.initZone()
	s.initProvider()
	s.initRedis()
	s.initDatabase()
	s.initModelCache()
	s.initRegistry()
	s.initRateLimit()
	s.initToken()
	s.initOtel()
	s.initTrace()
	s.initMetrics()
	return s.initMigration()
}

// initZone 初始化时区
func (s *gameServer) initZone() {
	time.Local = lo.Must(time.LoadLocation(viper.GetString("zone")))
}

// initProvider 初始化服务提供者
func (s *gameServer) initProvider() {
	serviceNames := viper.GetStringSlice("services")
	var services []boot.Service
	services = append(services, user.UserService)
	for _, service := range services {
		if lo.Contains(serviceNames, service.Name()) {
			boot.Register(service)
		}
	}
}

// initOtel 初始化 Otel
func (s *gameServer) initOtel() {
	otel.SetLogger(logr.New(otellog.New(8)))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Error().Err(err).Msg("otel error")
	}))
}

// initTrace 初始化链路追踪
func (s *gameServer) initTrace() {
	if !viper.GetBool("trace.enabled") {
		log.Info().Msg("trace disabled")
		return
	}
	info := boot.Read()

	endpoint := viper.GetString("trace.endpoint")
	exp := lo.Must(otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithTimeout(time.Second*5)))

	tp := traceSdk.NewTracerProvider(
		traceSdk.WithSampler(traceSdk.ParentBased(traceSdk.TraceIDRatioBased(1.0))),
		traceSdk.WithBatcher(exp),
		traceSdk.WithResource(resource.NewSchemaless(
			semconv.ServiceNameKey.String(info.Name),
			semconv.ServiceVersionKey.String(info.Version),
			semconv.ServiceInstanceIDKey.String(info.Hostname+"."+info.Name),
			attribute.Key("env").String(viper.GetString("env")),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	s.mu.Lock()
	s.tracerProvider = tp
	s.mu.Unlock()
}

// initMetrics 初始化指标，暂时不需要
func (s *gameServer) initMetrics() {
	if !viper.GetBool("metrics.enabled") {
		log.Info().Msg("metrics disabled")
		return
	}
}

// initRedis 初始化 Redis
func (s *gameServer) initRedis() {
	for key := range viper.GetStringMap("redis") {
		sub := viper.Sub("redis." + key)
		if sub == nil {
			continue
		}

		for _, db := range sub.GetIntSlice("dbs") {
			client := redis.NewClient(&redis.Options{
				Addr:        sub.GetString("addr"),
				Password:    sub.GetString("password"),
				DB:          db,
				DialTimeout: time.Second * 3,
			})

			err := redisotel.InstrumentTracing(client)
			if err != nil {
				log.Fatal().Err(err).Str("key", key).Int("db", db).Msg("instrument tracing error")
				continue
			}
			err = redisotel.InstrumentMetrics(client)
			if err != nil {
				log.Fatal().Err(err).Str("key", key).Int("db", db).Msg("instrument metrics error")
				continue
			}

			s.rdbs.Set(key, db, client)
			log.Info().Str("key", key).Int("db", db).Msg("init redis")
		}
	}
}

// initRegistry 初始化服务注册中心
func (s *gameServer) initRegistry() {
	if !viper.GetBool("registry.enabled") {
		return
	}
	s.mu.Lock()
	s.registry = redlb.NewRegistry(s.rdbs.Default())
	s.mu.Unlock()
}

// initToken 初始化Token管理器
func (s *gameServer) initToken() {
	var manOpts []token.Option
	if tokenExpires := viper.GetDuration("token.man.token_ttl"); tokenExpires > 0 {
		manOpts = append(manOpts, token.WithAccessTtl(tokenExpires))
	}
	if refreshExpires := viper.GetDuration("token.man.refresh_ttl"); refreshExpires > 0 {
		manOpts = append(manOpts, token.WithRefreshTtl(refreshExpires))
	}

	var userOpts []token.Option
	if tokenExpires := viper.GetDuration("token.user.token_ttl"); tokenExpires > 0 {
		userOpts = append(userOpts, token.WithAccessTtl(tokenExpires))
	}
	if refreshExpires := viper.GetDuration("token.user.refresh_ttl"); refreshExpires > 0 {
		userOpts = append(userOpts, token.WithRefreshTtl(refreshExpires))
	}

	s.mu.Lock()
	s.manToken = token.NewManToken(s.rdbs.MustGet("token", 0), manOpts...)
	s.userToken = token.NewUserToken(s.rdbs.MustGet("token", 0), userOpts...)
	s.mu.Unlock()
}

// initRateLimit 初始化限流管理器
func (s *gameServer) initRateLimit() {
	if !viper.GetBool("ratelimit.enabled") {
		log.Info().Msg("rate limit disabled")
		return
	}

	s.mu.Lock()
	s.rateLimiter = ratelimit.NewRateLimiter(s.rdbs.Default())
	s.mu.Unlock()

	log.Info().Msg("rate limit initialized")
}

// initDatabase 初始化数据库
func (s *gameServer) initDatabase() {
	dsn := viper.GetString("database.dsn")
	slow := viper.GetDuration("database.slow")

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	s.mu.Lock()
	s.db = bun.NewDB(sqldb, pgdialect.New())
	s.mu.Unlock()
	s.db.AddQueryHook(bunlog.NewQueryHook(bunlog.WithLogSlow(slow)))
}

// initModelCache 初始化模型缓存
func (s *gameServer) initModelCache() {
}

func (s *gameServer) initMigration() (err error) {
	last := strings.ToLower(os.Args[len(os.Args)-1])
	ctx := context.Background()

	if last != "up" && last != "down" && last != "init" && last != "fixture" {
		return
	}
	defer os.Exit(0)

	// 生产环境禁止回滚
	if viper.GetString("mode") != "debug" {
		if last == "down" {
			return errors.New("can't rollback in production")
		}
		if last == "fixture" {
			return errors.New("can't load fixture in production")
		}
	}
	if last == "fixture" {
		s.db.RegisterModel((*models.User)(nil))
		fixture := dbfixture.New(s.db, dbfixture.WithTruncateTables())
		var files []string
		filepath.WalkDir(viper.GetString("fixture.dir"), func(path string, d os.DirEntry, err error) error {
			if !d.IsDir() && filepath.Ext(path) == ".yaml" {
				files = append(files, path)
			}
			return err
		})
		err = fixture.Load(ctx, os.DirFS("."), files...)
		if err != nil {
			log.Fatal().Err(err).Msg("load fixture failed")
		}

		return
	}

	migrator := migrate.NewMigrator(s.db, migrations.Migrations)
	if last == "init" {
		migrator.Init(ctx)
		return
	}

	err = migrator.Lock(ctx)
	if err != nil {
		return
	}
	defer migrator.Unlock(ctx) //nolint:errcheck

	var group *migrate.MigrationGroup
	if last == "up" {
		group, err = migrator.Migrate(ctx)
	} else {
		group, err = migrator.Rollback(ctx)
	}
	if err != nil {
		return
	}
	if group.IsZero() {
		log.Error().Msg("no migration")
		return
	}
	log.Info().Str("op", last).Int("count", len(group.Migrations)).Msgf("migrate %v success", group)
	return
}
