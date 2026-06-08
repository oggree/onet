package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server"
	"github.com/kardianos/service"
)

type program struct {
	exit       chan struct{}
	cfStopChan chan struct{}
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		log.Println("Running in interactive mode.")
	} else {
		log.Println("Running under service manager.")
	}
	p.exit = make(chan struct{})
	p.cfStopChan = make(chan struct{})

	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}

func getPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(ip)), nil
}

func (p *program) checkIP() {
	ip, err := getPublicIP()
	if err != nil {
		log.Printf("Could not get public IP: %v", err)
	} else {
		log.Printf("Machine Public IP address: %s", ip)
	}
}

func startFRP(cfg Config) {
	frpCfg := &v1.ServerConfig{}
	frpCfg.BindPort = cfg.FRPBindPort
	frpCfg.VhostHTTPPort = cfg.FRPVhostHTTPPort
	frpCfg.Auth.Method = "token"
	frpCfg.Auth.Token = cfg.FRPAuthToken

	// Add Webhook Auth Plugin
	frpCfg.HTTPPlugins = []v1.HTTPPluginOptions{
		{
			Name: "onet_auth",
			Addr: "127.0.0.1:7001",
			Path: "/handler",
			Ops:  []string{"Login"},
		},
	}

	frpCfg.Complete()

	svr, err := server.NewService(frpCfg)
	if err != nil {
		log.Printf("Failed to create embedded FRP server: %v", err)
		return
	}

	log.Printf("Starting embedded FRP Server on BindPort %d, VhostHTTPPort %d", cfg.FRPBindPort, cfg.FRPVhostHTTPPort)
	svr.Run(context.Background())
}

func (p *program) run() {
	if err := initDB(); err != nil {
		log.Printf("Warning: Failed to initialize SQLite Database: %v", err)
	} else {
		log.Println("SQLite Database initialized at /etc/onet/onet.db")
	}

	if err := initConfig(); err != nil {
		log.Printf("Warning: Failed to initialize config: %v", err)
	} else {
		cfgMutex.RLock()
		PrintConfig(appConfig)
		isSet := appConfig.IsSet()
		cfgMutex.RUnlock()

		// Always start the internal web server for the setup UI
		go startInternalWebServer()

		if isSet {
			p.bootServices()
		} else {
			log.Println("App is not set up! Waiting for initial configuration via Setup Wizard (localhost:14080)...")
		}
	}

	// Check immediately on start
	p.checkIP()

	// Then check every 1 minute
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ticker.C:
			checkConfigChanges()
			p.checkIP()
		case <-p.exit:
			ticker.Stop()
			return
		}
	}
}

func (p *program) Stop(s service.Service) error {
	log.Println("Stopping service...")
	close(p.cfStopChan)
	close(p.exit)
	return nil
}

var (
	globalProgram *program
	Version       = "v0.1.0-alpha"
)

func (p *program) bootServices() {
	cfgMutex.RLock()
	cfg := appConfig
	cfgMutex.RUnlock()

	go startWebhookServer()
	go startFRP(cfg)
	time.Sleep(2 * time.Second)
	go startInternalFRPC(cfg.FRPAuthToken, cfg.FRPBindPort, "arc."+cfg.CFDomain)

	if cfg.CFApiToken != "" {
		log.Println("Cloudflare credentials detected. Setting up zero trust tunnel...")
		token, err := SetupCloudflareTunnel()
		if err != nil {
			log.Printf("Failed to setup Cloudflare tunnel via API: %v", err)
		} else if token != "" {
			go StartCloudflared(token, p.cfStopChan)
		}
	}
}

func main() {
	svcConfig := &service.Config{
		Name:        "onet",
		DisplayName: "Onet Service",
		Description: "This is the Onet background service.",
		Arguments:   []string{"orun"},
	}

	prg := &program{
		exit:       make(chan struct{}),
		cfStopChan: make(chan struct{}),
	}
	globalProgram = prg
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		action := os.Args[1]

		if action == "version" || action == "-v" || action == "--version" {
			fmt.Printf("Onet Version: %s\n", Version)
			return
		}

		// "orun" or "run" runs the app in the foreground without daemonizing
		if action == "orun" || action == "run" {
			log.Println("Running application in foreground...")
			err = s.Run()
			if err != nil {
				log.Fatal(err)
			}
			return
		}

		if action == "uninstall" {
			_ = s.Stop()
			err = s.Uninstall()
			if err != nil {
				log.Fatalf("Failed to uninstall service: %v\n(Run with sudo?)", err)
			}
			log.Println("Service uninstalled successfully.")
			return
		}

		// Token Management Commands
		if action == "token" {
			if len(os.Args) < 3 {
				fmt.Println("Usage: onet token [add <org> <token> | remove <token> | list]")
				return
			}
			cmd := os.Args[2]
			if err := initDB(); err != nil {
				log.Fatalf("Failed to open DB: %v", err)
			}

			switch cmd {
			case "add":
				if len(os.Args) < 5 {
					fmt.Println("Usage: onet token add <org> <token>")
					return
				}
				org := os.Args[3]
				token := os.Args[4]
				if err := AddToken(org, token); err != nil {
					log.Fatalf("Failed to add token: %v", err)
				}
				fmt.Printf("Added token for organization '%s' successfully.\n", org)
			case "remove":
				if len(os.Args) < 4 {
					fmt.Println("Usage: onet token remove <token>")
					return
				}
				token := os.Args[3]
				if err := RemoveToken(token); err != nil {
					log.Fatalf("Failed to remove token: %v", err)
				}
				fmt.Println("Token removed successfully.")
			case "list":
				tokens, err := ListTokens()
				if err != nil {
					log.Fatalf("Failed to list tokens: %v", err)
				}
				fmt.Println("ID\tOrganization\tToken")
				for _, t := range tokens {
					fmt.Printf("%d\t%s\t%s\n", t.ID, t.Organization, t.Token)
				}
			default:
				fmt.Println("Unknown token command:", cmd)
			}
			return
		}

		err = service.Control(s, action)
		if err != nil {
			log.Printf("Failed to perform action %q: %v\n", action, err)
			log.Printf("Valid actions: %q, orun, run\n", service.ControlAction)
		}
		return
	}

	// If no arguments, check if we're running interactively
	if service.Interactive() {
		log.Println("First start detected. Installing and starting as a system service...")
		err = s.Install()
		if err != nil {
			log.Printf("Failed to install service: %v\n(Make sure to run with sudo!)", err)
			return
		}
		log.Println("Service installed successfully.")

		err = s.Start()
		if err != nil {
			log.Printf("Failed to start service: %v", err)
			return
		}
		log.Println("Service started successfully.")
		return
	}

	// Running under the service manager (e.g., systemd)
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
