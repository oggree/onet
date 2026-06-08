package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func startInternalFRPC(masterToken string, bindPort int, customDomain string) {
	commonCfg := &v1.ClientCommonConfig{}
	commonCfg.ServerAddr = "127.0.0.1"
	commonCfg.ServerPort = bindPort
	commonCfg.Auth.Method = "token"
	commonCfg.Auth.Token = masterToken

	// Need to initialize common defaults (e.g. loginFailExit=true)
	commonCfg.Complete()
	falseValue := false
	commonCfg.LoginFailExit = &falseValue

	proxyCfg := &v1.HTTPProxyConfig{}
	proxyCfg.Name = "arc_dashboard"
	proxyCfg.Type = "http"
	proxyCfg.LocalIP = "127.0.0.1"
	proxyCfg.LocalPort = 14080
	proxyCfg.CustomDomains = []string{customDomain}
	proxyCfg.Complete()

	// Create Source Aggregator
	cfgSource := source.NewConfigSource()
	if err := cfgSource.ReplaceAll([]v1.ProxyConfigurer{proxyCfg}, nil); err != nil {
		log.Printf("Internal FRPC config error: %v", err)
		return
	}

	aggregator := source.NewAggregator(cfgSource)

	options := client.ServiceOptions{
		Common:                 commonCfg,
		ConfigSourceAggregator: aggregator,
	}

	svr, err := client.NewService(options)
	if err != nil {
		log.Printf("Internal FRPC init error: %v", err)
		return
	}

	log.Printf("Starting Internal FRP Client routing %s to localhost:14080...", customDomain)
	if err := svr.Run(context.Background()); err != nil {
		fmt.Printf("Internal FRPC exited: %v\n", err)
	}
}
