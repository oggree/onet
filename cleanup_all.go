//go:build ignore
// +build ignore

package main

import (
	"context"
	"io/ioutil"
	"log"

	"github.com/cloudflare/cloudflare-go"
	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey           string `yaml:"api_key"`
	Environment      string `yaml:"environment"`
	Port             int    `yaml:"port"`
	FRPBindPort      int    `yaml:"frp_bind_port"`
	FRPVhostHTTPPort int    `yaml:"frp_vhost_http_port"`
	FRPAuthToken     string `yaml:"frp_auth_token"`
	CFApiToken       string `yaml:"cf_api_token"`
	CFAccountID      string `yaml:"cf_account_id"`
	CFZoneID         string `yaml:"cf_zone_id"`
	CFDomain         string `yaml:"cf_domain"`
	CFTunnelID       string `yaml:"cf_tunnel_id"`
	CFTunnelSecret   string `yaml:"cf_tunnel_secret"`
}

func main() {
	configPath := "/etc/onet.yaml"
	log.Println("Reading configuration from", configPath)
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	token := cfg.CFApiToken
	domain := cfg.CFDomain

	if token == "" || domain == "" {
		log.Println("Warning: CFApiToken or CFDomain is already empty in config.")
		log.Println("Proceeding with config reset...")
	} else {
		log.Printf("Token found. Accessing Cloudflare API for domain: %s...", domain)

		api, err := cloudflare.NewWithAPIToken(token)
		if err != nil {
			log.Fatalf("Failed to create Cloudflare API client: %v", err)
		}

		ctx := context.Background()

		// 1. Fetch Zone
		zones, err := api.ListZones(ctx, domain)
		if err != nil {
			log.Printf("[-] ListZones failed: %v", err)
		} else if len(zones) == 0 {
			log.Printf("[-] No zone found for domain %s", domain)
		} else {
			zone := zones[0]
			zoneRC := cloudflare.ZoneIdentifier(zone.ID)

			// 2. Clean DNS records
			log.Println("Listing DNS Records to delete wildcard and arc configurations...")
			records, _, err := api.ListDNSRecords(ctx, zoneRC, cloudflare.ListDNSRecordsParams{})
			if err != nil {
				log.Printf("[-] Failed to list DNS records: %v", err)
			} else {
				for _, r := range records {
					isWildcard := r.Name == "*."+domain
					isArc := r.Name == "arc."+domain
					if isWildcard || isArc {
						log.Printf("[!] Deleting DNS record: Type=%s, Name=%s, Content=%s", r.Type, r.Name, r.Content)
						err := api.DeleteDNSRecord(ctx, zoneRC, r.ID)
						if err != nil {
							log.Printf("  [-] Failed to delete record %s: %v", r.ID, err)
						} else {
							log.Printf("  [+] Deleted DNS record successfully")
						}
					}
				}
			}

			// 3. Clean Cloudflare Tunnels
			log.Println("Listing Cloudflare Tunnels to delete onet-embedded-tunnel...")
			accountRC := cloudflare.ResourceIdentifier(zone.Account.ID)
			tunnels, _, err := api.ListTunnels(ctx, accountRC, cloudflare.TunnelListParams{})
			if err != nil {
				log.Printf("[-] Failed to list tunnels: %v", err)
			} else {
				for _, t := range tunnels {
					if t.Name == "onet-embedded-tunnel" {
						log.Printf("[!] Deleting Cloudflare Tunnel: Name=%s, ID=%s", t.Name, t.ID)
						err := api.DeleteTunnel(ctx, accountRC, t.ID)
						if err != nil {
							log.Printf("  [-] Failed to delete tunnel %s: %v (it might still have connections or be active)", t.ID, err)
						} else {
							log.Printf("  [+] Deleted tunnel successfully")
						}
					}
				}
			}
		}
	}

	// 4. Reset config file
	log.Println("Resetting /etc/onet.yaml configuration...")
	defaultCfg := Config{
		APIKey:           "default-key-change-me",
		Environment:      "production",
		Port:             8080,
		FRPBindPort:      7000,
		FRPVhostHTTPPort: 8080,
		FRPAuthToken:     "change-this-token",
		CFApiToken:       "",
		CFAccountID:      "",
		CFZoneID:         "",
		CFDomain:         "",
		CFTunnelID:       "",
		CFTunnelSecret:   "",
	}

	yamlBytes, err := yaml.Marshal(defaultCfg)
	if err != nil {
		log.Fatalf("Failed to marshal default config: %v", err)
	}

	err = ioutil.WriteFile(configPath, yamlBytes, 0600)
	if err != nil {
		log.Fatalf("Failed to write default config: %v", err)
	}

	log.Println("[+] /etc/onet.yaml successfully reset to defaults!")
	log.Println("All done! You can now run 'sudo ./bin/onet orun' to test the onboarding wizard from localhost:14080.")
}
