package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// GetAuthKey computes the FRP privilege key.
func GetAuthKey(token string, timestamp int64) string {
	md5Ctx := md5.New()
	md5Ctx.Write([]byte(token))
	md5Ctx.Write([]byte(strconv.FormatInt(timestamp, 10)))
	return hex.EncodeToString(md5Ctx.Sum(nil))
}

type WebhookReq struct {
	Op      string                 `json:"op"`
	Content map[string]interface{} `json:"content"`
}

type WebhookResp struct {
	Reject       bool                   `json:"reject"`
	RejectReason string                 `json:"reject_reason"`
	Unchange     bool                   `json:"unchange"`
	Content      map[string]interface{} `json:"content"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	var req WebhookReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	reqBytes, _ := json.Marshal(req)
	log.Printf("FRP Webhook Request: %s", string(reqBytes))

	resp := WebhookResp{Reject: true, RejectReason: "invalid token", Unchange: true}

	if req.Op == "Login" {
		if req.Content == nil {
			req.Content = make(map[string]interface{})
		}
		content := req.Content

		// Extract fields
		timestampFloat, _ := content["timestamp"].(float64)
		timestamp := int64(timestampFloat)
		privilegeKey, _ := content["privilege_key"].(string)
		user, _ := content["user"].(string)

		cfgMutex.RLock()
		masterToken := appConfig.FRPAuthToken
		cfgMutex.RUnlock()

		if GetAuthKey(masterToken, timestamp) == privilegeKey {
			resp.Reject = false
			resp.RejectReason = ""
			resp.Unchange = true
			sendWebhookResp(w, resp)
			return
		}

		// 2. Check SQLite Database
		tokens, err := ListTokens()
		if err == nil {
			for _, t := range tokens {
				// If user is provided, we can optimize by only checking matching organization
				if user != "" && t.Organization != user {
					continue
				}

				if GetAuthKey(t.Token, timestamp) == privilegeKey {
					resp.Reject = false
					resp.RejectReason = ""
					resp.Unchange = false
					req.Content["privilege_key"] = GetAuthKey(masterToken, timestamp)
					resp.Content = req.Content
					log.Printf("FRP Auth successful via SQLite token for organization: %s", t.Organization)
					sendWebhookResp(w, resp)
					return
				}
			}
		}

		log.Printf("FRP Auth rejected: invalid token for user '%s'", user)
	} else {
		// Allow all other operations (e.g. Ping, NewProxy)
		resp.Reject = false
		resp.RejectReason = ""
	}

	sendWebhookResp(w, resp)
}

func sendWebhookResp(w http.ResponseWriter, resp WebhookResp) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func startWebhookServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/handler", webhookHandler)

	log.Println("Starting internal FRP Webhook Server on 127.0.0.1:7001...")
	if err := http.ListenAndServe("127.0.0.1:7001", mux); err != nil {
		log.Printf("Webhook server error: %v", err)
	}
}
