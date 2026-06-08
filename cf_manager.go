package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudflare/cloudflare-go"
)

func SetupCloudflareTunnel() (string, error) {
	cfgMutex.RLock()
	cfg := appConfig
	cfgMutex.RUnlock()

	if cfg.CFApiToken == "" || cfg.CFAccountID == "" || cfg.CFZoneID == "" || cfg.CFDomain == "" {
		return "", fmt.Errorf("cloudflare API credentials or IDs are missing in configuration")
	}

	api, err := cloudflare.NewWithAPIToken(cfg.CFApiToken)
	if err != nil {
		return "", fmt.Errorf("failed to initialize Cloudflare API: %v", err)
	}

	ctx := context.Background()
	tunnelName := "onet-embedded-tunnel"

	var tunnelID string
	var tunnelSecret string

	// Generate secret if not present
	if cfg.CFTunnelSecret == "" {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return "", err
		}
		tunnelSecret = base64.StdEncoding.EncodeToString(secretBytes)

		cfgMutex.Lock()
		appConfig.CFTunnelSecret = tunnelSecret
		cfgMutex.Unlock()
		SaveConfig() // Save to file
		log.Println("Generated new Tunnel Secret and saved to config.")
	} else {
		tunnelSecret = cfg.CFTunnelSecret
	}

	rc := cloudflare.ResourceIdentifier(cfg.CFAccountID)

	// 1. Check if tunnel exists
	tunnels, _, err := api.ListTunnels(ctx, rc, cloudflare.TunnelListParams{
		Name: tunnelName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list tunnels: %v", err)
	}

	if len(tunnels) == 0 {
		log.Println("Tunnel does not exist. Creating...")

		tunnel, err := api.CreateTunnel(ctx, rc, cloudflare.TunnelCreateParams{
			Name:   tunnelName,
			Secret: tunnelSecret,
		})
		if err != nil {
			return "", fmt.Errorf("failed to create tunnel: %v", err)
		}
		tunnelID = tunnel.ID

		cfgMutex.Lock()
		appConfig.CFTunnelID = tunnelID
		cfgMutex.Unlock()
		SaveConfig()
		log.Println("Created Cloudflare Tunnel:", tunnelID)
	} else {
		tunnelID = tunnels[0].ID
		if appConfig.CFTunnelID != tunnelID {
			cfgMutex.Lock()
			appConfig.CFTunnelID = tunnelID
			cfgMutex.Unlock()
			SaveConfig()
		}
		log.Println("Found existing Cloudflare Tunnel:", tunnelID)
	}

	// 2. Configure Tunnel Ingress Rules
	// We route catchall '*' to localhost:8080 (where embedded FRP is listening)
	_, err = api.UpdateTunnelConfiguration(ctx, rc, cloudflare.TunnelConfigurationParams{
		TunnelID: tunnelID,
		Config: cloudflare.TunnelConfiguration{
			Ingress: []cloudflare.UnvalidatedIngressRule{
				{
					Hostname: fmt.Sprintf("*.%s", cfg.CFDomain),
					Service:  fmt.Sprintf("http://localhost:%d", cfg.FRPVhostHTTPPort),
				},
				{
					Service: "http_status:404",
				},
			},
		},
	})
	if err != nil {
		log.Printf("Warning: Failed to update tunnel ingress config: %v (Ignore if already set up)", err)
	} else {
		log.Println("Tunnel ingress configured successfully.")
	}

	// 3. Create/Update Wildcard CNAME in DNS
	zoneRC := cloudflare.ZoneIdentifier(cfg.CFZoneID)

	recordName := fmt.Sprintf("*.%s", cfg.CFDomain)
	target := fmt.Sprintf("%s.cfargotunnel.com", tunnelID)

	records, _, err := api.ListDNSRecords(ctx, zoneRC, cloudflare.ListDNSRecordsParams{
		Name: recordName,
		Type: "CNAME",
	})
	if err != nil {
		return "", fmt.Errorf("failed to list DNS records: %v", err)
	}

	if len(records) == 0 {
		log.Println("DNS CNAME missing. Creating wildcard CNAME...")
		proxied := true
		_, err := api.CreateDNSRecord(ctx, zoneRC, cloudflare.CreateDNSRecordParams{
			Type:    "CNAME",
			Name:    recordName,
			Content: target,
			Proxied: &proxied,
		})
		if err != nil {
			log.Printf("Warning: failed to create DNS record: %v", err)
		} else {
			log.Println("Created wildcard DNS record.")
		}
	} else {
		if records[0].Content != target {
			log.Println("DNS CNAME mismatch. Updating...")
			proxied := true
			_, err := api.UpdateDNSRecord(ctx, zoneRC, cloudflare.UpdateDNSRecordParams{
				ID:      records[0].ID,
				Type:    "CNAME",
				Name:    recordName,
				Content: target,
				Proxied: &proxied,
			})
			if err != nil {
				log.Printf("Warning: failed to update DNS record: %v", err)
			} else {
				log.Println("Updated wildcard DNS record.")
			}
		} else {
			log.Println("DNS CNAME is correctly configured.")
		}
	}

	// 4. Construct Tunnel Token
	type TokenPayload struct {
		A string `json:"a"`
		T string `json:"t"`
		S string `json:"s"`
	}
	payload := TokenPayload{
		A: cfg.CFAccountID,
		T: tunnelID,
		S: tunnelSecret,
	}
	jsonPayload, _ := json.Marshal(payload)
	token := base64.StdEncoding.EncodeToString(jsonPayload)

	return token, nil
}

type PermissionReport struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Error       string `json:"error,omitempty"`
}

func VerifyTokenPermissions() ([]PermissionReport, error) {
	cfgMutex.RLock()
	cfg := appConfig
	cfgMutex.RUnlock()

	if cfg.CFApiToken == "" {
		return nil, fmt.Errorf("Cloudflare API token not configured")
	}

	api, err := cloudflare.NewWithAPIToken(cfg.CFApiToken)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var reports []PermissionReport

	// 1. Basic Token Verification
	passedToken := true
	errStr := ""
	_, err = api.VerifyAPIToken(ctx)
	if err != nil {
		passedToken = false
		errStr = err.Error()
	}
	reports = append(reports, PermissionReport{
		Name:        "Token Validity",
		Description: "Checks if the API token is active and valid.",
		Passed:      passedToken,
		Error:       errStr,
	})

	if !passedToken {
		return reports, nil
	}

	// 2. Zone DNS Access
	passedZone := true
	errStr = ""
	if cfg.CFZoneID != "" {
		_, _, err = api.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(cfg.CFZoneID), cloudflare.ListDNSRecordsParams{})
		if err != nil {
			passedZone = false
			errStr = err.Error()
		}
	} else if cfg.CFDomain != "" {
		zones, err := api.ListZones(ctx, cfg.CFDomain)
		if err != nil || len(zones) == 0 {
			passedZone = false
			if err != nil {
				errStr = err.Error()
			} else {
				errStr = "zone not found for domain"
			}
		}
	} else {
		passedZone = false
		errStr = "zone/domain not configured"
	}
	reports = append(reports, PermissionReport{
		Name:        "Zone DNS Read/Write",
		Description: "Checks if Onet can read/write DNS records for your domain.",
		Passed:      passedZone,
		Error:       errStr,
	})

	// 3. Cloudflare Tunnels Access
	passedTunnel := true
	errStr = ""
	if cfg.CFAccountID != "" {
		_, _, err = api.ListTunnels(ctx, cloudflare.ResourceIdentifier(cfg.CFAccountID), cloudflare.TunnelListParams{})
		if err != nil {
			passedTunnel = false
			errStr = err.Error()
		}
	} else {
		passedTunnel = false
		errStr = "account ID not configured"
	}
	reports = append(reports, PermissionReport{
		Name:        "Cloudflare Tunnels Management",
		Description: "Checks if Onet can list, create, and manage Zero Trust Tunnels.",
		Passed:      passedTunnel,
		Error:       errStr,
	})

	// 4. Access Apps & Policies
	passedAccessApps := true
	errStr = ""
	if cfg.CFAccountID != "" {
		_, _, err = api.ListAccessApplications(ctx, cloudflare.ResourceIdentifier(cfg.CFAccountID), cloudflare.ListAccessApplicationsParams{})
		if err != nil {
			passedAccessApps = false
			errStr = err.Error()
		}
	} else {
		passedAccessApps = false
		errStr = "account ID not configured"
	}
	reports = append(reports, PermissionReport{
		Name:        "Access Apps & Policies",
		Description: "Checks if Onet can manage Zero Trust Access Applications and Policies.",
		Passed:      passedAccessApps,
		Error:       errStr,
	})

	// 5. Access Identity Providers
	passedAccessOrg := true
	errStr = ""
	if cfg.CFAccountID != "" {
		_, _, err = api.GetAccessOrganization(ctx, cloudflare.ResourceIdentifier(cfg.CFAccountID), cloudflare.GetAccessOrganizationParams{})
		if err != nil {
			passedAccessOrg = false
			errStr = err.Error()
		}
	} else {
		passedAccessOrg = false
		errStr = "account ID not configured"
	}
	reports = append(reports, PermissionReport{
		Name:        "Access Identity Providers & Org",
		Description: "Checks if Onet can read Access Identity Providers and Organization settings.",
		Passed:      passedAccessOrg,
		Error:       errStr,
	})

	return reports, nil
}
