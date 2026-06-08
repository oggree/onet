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
	log.Println("Reading configuration from /etc/onet.yaml...")
	data, err := ioutil.ReadFile("/etc/onet.yaml")
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	if cfg.CFApiToken == "" {
		log.Fatalf("Cloudflare API Token not found in configuration")
	}

	log.Printf("Domain: %s", cfg.CFDomain)

	api, err := cloudflare.NewWithAPIToken(cfg.CFApiToken)
	if err != nil {
		log.Fatalf("Failed to create Cloudflare API client: %v", err)
	}

	ctx := context.Background()

	// 1. Verify Token
	log.Println("Checking Token Validity...")
	tokenVer, err := api.VerifyAPIToken(ctx)
	if err != nil {
		log.Printf("[-] Token Verification Failed: %v", err)
	} else {
		log.Printf("[+] Token Verification Passed: status=%s", tokenVer.Status)
	}

	// 2. Fetch Zone
	log.Printf("Fetching Zone for domain %s...", cfg.CFDomain)
	zones, err := api.ListZones(ctx, cfg.CFDomain)
	if err != nil {
		log.Printf("[-] ListZones failed: %v", err)
		return
	}
	if len(zones) == 0 {
		log.Printf("[-] No zone found for domain %s", cfg.CFDomain)
		return
	}
	zone := zones[0]
	log.Printf("[+] Zone Found: ID=%s, Name=%s, AccountID=%s", zone.ID, zone.Name, zone.Account.ID)

	zoneRC := cloudflare.ZoneIdentifier(zone.ID)

	// 3. List and Clean conflicting DNS Records
	log.Println("Listing DNS Records for the zone to find conflicts...")
	records, _, err := api.ListDNSRecords(ctx, zoneRC, cloudflare.ListDNSRecordsParams{})
	if err != nil {
		log.Printf("[-] Failed to list DNS records: %v", err)
	} else {
		log.Printf("[+] Found %d DNS records in zone", len(records))
		for _, r := range records {
			log.Printf("  Record: ID=%s, Type=%s, Name=%s, Content=%s", r.ID, r.Type, r.Name, r.Content)

			// Let's identify conflicting wildcard or specific records
			isWildcard := r.Name == "*."+cfg.CFDomain
			isArc := r.Name == "arc."+cfg.CFDomain

			if isWildcard || isArc {
				// If it is an A record or any record that is not CNAME to our tunnel, delete it
				if r.Type == "A" || r.Type == "AAAA" {
					log.Printf("    [!] Conflict found: %s record for %s pointing to %s. Deleting...", r.Type, r.Name, r.Content)
					err := api.DeleteDNSRecord(ctx, zoneRC, r.ID)
					if err != nil {
						log.Printf("    [-] Failed to delete record %s: %v", r.ID, err)
					} else {
						log.Printf("    [+] Successfully deleted record %s", r.ID)
					}
				}
			}
		}
	}

	// 4. List Tunnels
	log.Println("Checking Cloudflare Tunnels...")
	accountRC := cloudflare.ResourceIdentifier(zone.Account.ID)
	tunnels, _, err := api.ListTunnels(ctx, accountRC, cloudflare.TunnelListParams{})
	if err != nil {
		log.Printf("[-] Failed to list tunnels: %v", err)
	} else {
		log.Printf("[+] Found %d tunnels for account", len(tunnels))
		for _, t := range tunnels {
			log.Printf("  Tunnel: ID=%s, Name=%s, Status=%s", t.ID, t.Name, t.Status)
			if t.Name == "onet-embedded-tunnel" {
				log.Printf("    Found active onet-embedded-tunnel. ID: %s", t.ID)
			}
		}
	}

	log.Println("Cleanup & Verification complete!")
}
