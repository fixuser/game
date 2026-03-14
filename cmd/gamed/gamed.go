package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/game/game/gamed"
	"github.com/game/game/pkg/boot"
	"github.com/rs/zerolog/log"
)

var (
	cfgName     string
	versionInfo bool
)

func main() {
	flag.BoolVar(&versionInfo, "v", false, "显示版本信息")
	flag.StringVar(&cfgName, "cfg", "", "配置文件路径")
	flag.Parse()

	if versionInfo {
		fmt.Println(boot.Read().String())
		return
	}

	opts := gamed.NewOption()
	if err := opts.Load(cfgName); err != nil {
		log.Fatal().Err(err).Msg("load config failed")
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	srv := gamed.New(opts)
	srv.Main()
	<-signalChan
	srv.Exit()
}
