// Package redlb 提供基于 Redis 的服务注册与服务发现，包含 gRPC resolver。
//
// 基本使用：
//
//	reg := redlb.NewRegistry(rdb)
//	lease, _, err := reg.Register(ctx, "user.service")
//	if err != nil {
//		panic(err)
//	}
//	defer lease.Stop(context.Background())
//
//	redlb.RegisterGrpcResolver(reg)
//	conn, err := grpc.Dial(
//		"redlb:///user.service",
//		grpc.WithTransportCredentials(insecure.NewCredentials()),
//		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
//	)
//
// 默认行为：
// - 从 boot.Read() 读取 Ip/Hostname。
// - 从 viper 读取 grpc.addr 与 http.addr 并提取端口。
// - 4 秒心跳续期，12 秒 Ttl。
package redlb
