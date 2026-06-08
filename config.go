package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"reflect"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKey           string `yaml:"api_key" onet:"secure"`
	Environment      string `yaml:"environment"`
	Port             int    `yaml:"port"`
	FRPBindPort      int    `yaml:"frp_bind_port"`
	FRPVhostHTTPPort int    `yaml:"frp_vhost_http_port"`
	FRPAuthToken     string `yaml:"frp_auth_token" onet:"secure"`
	CFApiToken       string `yaml:"cf_api_token" onet:"secure"`
	CFAccountID      string `yaml:"cf_account_id"`
	CFZoneID         string `yaml:"cf_zone_id"`
	CFDomain         string `yaml:"cf_domain"`
	CFTunnelID       string `yaml:"cf_tunnel_id"`
	CFTunnelSecret   string `yaml:"cf_tunnel_secret" onet:"secure"`
}

var (
	configPath  = "/etc/onet.yaml"
	currentHash string
	cfgMutex    sync.RWMutex
	appConfig   Config
)

func getDefaultConfig() Config {
	return Config{
		APIKey:           "default-key-change-me",
		Environment:      "production",
		Port:             8080,
		FRPBindPort:      7000,
		FRPVhostHTTPPort: 8080,
		FRPAuthToken:     "change-this-token",
		CFApiToken:       "",
		CFAccountID:      "",
		CFZoneID:         "",
		CFDomain:         "example.com",
		CFTunnelID:       "",
		CFTunnelSecret:   "",
	}
}

func PrintConfig(cfg Config) {
	log.Println("--- Current Configuration ---")
	v := reflect.ValueOf(cfg)
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		val := v.Field(i).Interface()

		keyName := field.Tag.Get("yaml")
		if keyName == "" {
			keyName = field.Name
		}

		if field.Tag.Get("onet") == "secure" {
			log.Printf("%s: ********", keyName)
		} else {
			log.Printf("%s: %v", keyName, val)
		}
	}
	log.Println("-----------------------------")
}

func initConfig() error {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		log.Println("Config file missing. Creating with defaults at", configPath)
		return writeConfig(getDefaultConfig())
	} else if err != nil {
		return err
	}

	currentHash = hashData(data)
	return parseAndMigrateConfig(data)
}

func writeConfig(cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// Write with 0600 permissions to protect the config file
	err = os.WriteFile(configPath, data, 0600)
	if err != nil {
		return err
	}
	currentHash = hashData(data)

	cfgMutex.Lock()
	appConfig = cfg
	cfgMutex.Unlock()
	return nil
}

func SaveConfig() error {
	cfgMutex.RLock()
	cfg := appConfig
	cfgMutex.RUnlock()
	return writeConfig(cfg)
}

func parseAndMigrateConfig(data []byte) error {
	var fileMap map[string]interface{}
	if err := yaml.Unmarshal(data, &fileMap); err != nil {
		return err
	}
	if fileMap == nil {
		fileMap = make(map[string]interface{})
	}

	defaultCfg := getDefaultConfig()
	defaultBytes, _ := yaml.Marshal(defaultCfg)
	var defaultMap map[string]interface{}
	yaml.Unmarshal(defaultBytes, &defaultMap)

	needsRewrite := false
	for k, v := range defaultMap {
		if _, exists := fileMap[k]; !exists {
			log.Printf("ERROR: Missing config key '%s'. Adding default value to config file.", k)
			fileMap[k] = v
			needsRewrite = true
		}
	}

	if needsRewrite {
		mergedBytes, err := yaml.Marshal(fileMap)
		if err != nil {
			return err
		}
		if err := os.WriteFile(configPath, mergedBytes, 0600); err != nil {
			return err
		}
		data = mergedBytes
		currentHash = hashData(data)
		log.Println("Config file updated with missing keys.")
	}

	var finalCfg Config
	if err := yaml.Unmarshal(data, &finalCfg); err != nil {
		return err
	}

	cfgMutex.Lock()
	appConfig = finalCfg
	cfgMutex.Unlock()

	return nil
}

func hashData(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func checkConfigChanges() {
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Failed to read config for hash check: %v", err)
		return
	}
	newHash := hashData(data)
	if newHash != currentHash {
		log.Println("Config file change detected! Reloading config...")
		if err := parseAndMigrateConfig(data); err != nil {
			log.Printf("Failed to reload config: %v", err)
		} else {
			log.Println("Config reloaded successfully.")

			// Let's print out the new config to prove it loaded
			cfgMutex.RLock()
			PrintConfig(appConfig)
			cfgMutex.RUnlock()
		}
	}
}

func (c *Config) IsSet() bool {
	return c.CFApiToken != "" && c.CFDomain != ""
}

func ClearConfig() error {
	defaultCfg := getDefaultConfig()
	defaultCfg.CFDomain = ""
	return writeConfig(defaultCfg)
}
