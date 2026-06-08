package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed web/*
var webFS embed.FS

func startInternalWebServer() {
	// Strip the "web" directory prefix from the embedded filesystem
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Printf("Failed to create sub filesystem for web server: %v", err)
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		cfgMutex.RLock()
		isSet := appConfig.IsSet()
		cfgMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		if isSet {
			w.Write([]byte(`{"is_set": true}`))
		} else {
			w.Write([]byte(`{"is_set": false}`))
		}
	})

	mux.HandleFunc("/api/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Domain string `json:"domain"`
			Token  string `json:"token"`
		}
		importJson := json.NewDecoder(r.Body).Decode(&payload)
		if importJson != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		cfgMutex.Lock()

		// Verify Cloudflare Token by fetching the Zone
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/zones?name="+payload.Domain, nil)
		req.Header.Add("Authorization", "Bearer "+payload.Token)
		req.Header.Add("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			cfgMutex.Unlock()
			http.Error(w, "Invalid Cloudflare Token or Network Error. Check permissions.", http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()

		var zoneResp struct {
			Success bool `json:"success"`
			Result  []struct {
				ID      string `json:"id"`
				Account struct {
					ID string `json:"id"`
				} `json:"account"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&zoneResp); err != nil || !zoneResp.Success || len(zoneResp.Result) == 0 {
			cfgMutex.Unlock()
			http.Error(w, "Could not find domain in Cloudflare. Does the token have Zone:DNS:Read permissions?", http.StatusForbidden)
			return
		}

		appConfig.CFZoneID = zoneResp.Result[0].ID
		appConfig.CFAccountID = zoneResp.Result[0].Account.ID
		appConfig.CFDomain = payload.Domain
		appConfig.CFApiToken = payload.Token
		cfgMutex.Unlock()

		if err := SaveConfig(); err != nil {
			http.Error(w, "Failed to save config", http.StatusInternalServerError)
			return
		}

		// Boot services dynamically
		if globalProgram != nil {
			globalProgram.bootServices()
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	})

	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ClearConfig()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	})

	mux.HandleFunc("/api/verify", func(w http.ResponseWriter, r *http.Request) {
		reports, err := VerifyTokenPermissions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reports)
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		cfgMutex.RLock()
		defer cfgMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		var resp = map[string]string{
			"domain":     appConfig.CFDomain,
			"account_id": appConfig.CFAccountID,
			"zone_id":    appConfig.CFZoneID,
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			tokens, err := ListTokens()
			if err != nil {
				// Return empty array instead of error if not loaded
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[]`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokens)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/tokens/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Organization string `json:"organization"`
			Token        string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Organization == "" || payload.Token == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := AddToken(payload.Organization, payload.Token); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	})

	mux.HandleFunc("/api/tokens/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Token == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := RemoveToken(payload.Token); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	})

	log.Println("Starting internal Arc dashboard on 127.0.0.1:14080...")
	if err := http.ListenAndServe("127.0.0.1:14080", basicAuth(mux)); err != nil {
		log.Printf("Internal web server error: %v", err)
	}
}

func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfgMutex.RLock()
		apiKey := appConfig.APIKey
		cfgMutex.RUnlock()

		if apiKey != "" {
			_, password, ok := r.BasicAuth()
			if !ok || password != apiKey {
				w.Header().Set("WWW-Authenticate", `Basic realm="Onet Dashboard"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

