package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
	"github.com/spf13/viper"
)

type Config struct {
	APIToken       string `mapstructure:"CF_API_TOKEN" env:"CF_API_TOKEN"`
	AccountID      string `mapstructure:"CF_ACCOUNT_ID" env:"CF_ACCOUNT_ID"`
	AdminUser      string `mapstructure:"ADMIN_USER" env:"ADMIN_USER"`
	AdminPass      string `mapstructure:"ADMIN_PASSWORD" env:"ADMIN_PASSWORD"`
	ListenPort     int    `mapstructure:"LISTEN_PORT" env:"LISTEN_PORT"`
	SessionSecret  string `mapstructure:"SESSION_SECRET" env:"SESSION_SECRET"`
}

type Tunnel struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	UUID        string    `json:"uuid"`
	AccountID   string    `json:"account_id"`
	ZoneID      string    `json:"zone_id,omitempty"`
	Subdomain   string    `json:"subdomain,omitempty"`
	Domain      string    `json:"domain,omitempty"`
	Address    string    `json:"address,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
	PID         int       `json:"pid,omitempty"`
}

type IngressRule struct {
	ID        int    `json:"id"`
	TunnelID  int    `json:"tunnel_id"`
	Hostname string `json:"hostname"`
	Path      string `json:"path,omitempty"`
	Service  string `json:"service"`
	Protocol string `json:"protocol,omitempty"`
}

type LogEntry struct {
	ID        int       `json:"id"`
	TunnelID  int       `json:"tunnel_id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

var (
	db         *sql.DB
	cfg        Config
	tunnelProcs = sync.Map{}
)

const (
	sessionCookieName = "cft_session"
	sessionMaxAge     = 7 * 24 * 3600 // 7 days
)

type sessionPayload struct {
	User string `json:"u"`
	Exp  int64  `json:"exp"`
}

func (c *Config) sessionKey() []byte {
	s := strings.TrimSpace(c.SessionSecret)
	if s != "" {
		return []byte(s)
	}
	sum := sha256.Sum256([]byte(c.AdminPass + "9cf-ui-session-v1"))
	return sum[:]
}

func signSession(username string) (string, error) {
	p := sessionPayload{
		User: username,
		Exp:  time.Now().Add(time.Duration(sessionMaxAge) * time.Second).Unix(),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, cfg.sessionKey())
	mac.Write(raw)
	sig := mac.Sum(nil)
	payloadB64 := base64.RawURLEncoding.EncodeToString(raw)
	sigHex := hex.EncodeToString(sig)
	return payloadB64 + "." + sigHex, nil
}

func verifySessionToken(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := hex.DecodeString(parts[1])
	if err != nil || len(sig) != 32 {
		return "", false
	}
	mac := hmac.New(sha256.New, cfg.sessionKey())
	mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", false
	}
	if time.Now().Unix() > p.Exp {
		return "", false
	}
	if p.User == "" {
		return "", false
	}
	return p.User, true
}

func sessionUserFromRequest(c *gin.Context) (string, bool) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie == "" {
		return "", false
	}
	return verifySessionToken(cookie)
}

func setSessionCookie(c *gin.Context, token string) {
	httpOnly := true
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func postLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if req.Username != cfg.AdminUser || req.Password != cfg.AdminPass {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}
	token, err := signSession(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create session"})
		return
	}
	setSessionCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"username": req.Username})
}

func postLogout(c *gin.Context) {
	clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

func getAuthMe(c *gin.Context) {
	u, exists := c.Get("user")
	if !exists || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	name, ok := u.(string)
	if !ok || name == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"username": name})
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// SPA (HTML + /assets) must load before login; client-side routes (/login, …) need GET without session.
		if c.Request.Method == http.MethodGet && !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		if path == "/api/login" && c.Request.Method == http.MethodPost {
			c.Next()
			return
		}
		if path == "/api/logout" && c.Request.Method == http.MethodPost {
			c.Next()
			return
		}

		if user, ok := sessionUserFromRequest(c); ok && user == cfg.AdminUser {
			c.Set("user", user)
			c.Next()
			return
		}

		authUser, authPass, hasAuth := c.Request.BasicAuth()
		if hasAuth && authUser == cfg.AdminUser && authPass == cfg.AdminPass {
			c.Set("user", cfg.AdminUser)
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
	}
}

func normalizeCFConfig(c *Config) {
	c.APIToken = strings.TrimSpace(strings.Trim(c.APIToken, `"'`))
	c.AccountID = strings.TrimSpace(strings.Trim(c.AccountID, `"'`))
}

func cfRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Timeout: 60 * time.Second}).Do(req)
}

// validateAccountIDForTunnelAPI checks that CF_ACCOUNT_ID is one of the accounts this token may use.
// Zone-only tokens (DNS only) often cannot create tunnels — listing accounts fails or omits the ID.
func validateAccountIDForTunnelAPI(accountID string) string {
	if accountID == "" || cfg.APIToken == "" {
		return ""
	}
	resp, err := cfRequest("GET", "https://api.cloudflare.com/client/v4/accounts?per_page=50", nil)
	if err != nil {
		return fmt.Sprintf(" Could not list Cloudflare accounts: %v.", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(b, &out)
	if resp.StatusCode >= 400 || !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return fmt.Sprintf(" Token cannot list accounts (HTTP %d). Add permission: Account → Account Settings → Read, or widen Account resources on the token. %s",
			resp.StatusCode, msg)
	}
	for _, a := range out.Result {
		if a.ID == accountID {
			return ""
		}
	}
	var ids []string
	for _, a := range out.Result {
		ids = append(ids, a.ID)
		if len(ids) >= 5 {
			break
		}
	}
	return fmt.Sprintf(" CF_ACCOUNT_ID does not match any account this token can access (first IDs: %v). Fix CF_ACCOUNT_ID in .env or edit the token so Account resources include that account (or All accounts).", ids)
}

func main() {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	viper.Unmarshal(&cfg)
	normalizeCFConfig(&cfg)

	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.AdminPass == "" {
		cfg.AdminPass = "changeme"
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 3000
	}

	if err := initDB(); err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	// Quiet stdout: tunnel/DNS helpers use log.Printf; UI still has /api/logs in SQLite.
	log.SetOutput(io.Discard)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.Use(authMiddleware())

	r.POST("/api/login", postLogin)
	r.POST("/api/logout", postLogout)
	r.GET("/api/auth/me", getAuthMe)

	r.GET("/api/tunnels", listTunnels)
	r.POST("/api/tunnels", createTunnel)
	r.GET("/api/tunnels/:id", getTunnel)
	r.DELETE("/api/tunnels/:id", deleteTunnel)
	r.POST("/api/tunnels/:id/start", startTunnel)
	r.POST("/api/tunnels/:id/stop", stopTunnel)
	r.GET("/api/tunnels/:id/logs", getTunnelLogs)
	r.GET("/api/logs", getAllLogs)

	r.GET("/api/ingress", listIngressRules)
	r.POST("/api/ingress", createIngressRule)
	r.PUT("/api/ingress/:id", updateIngressRule)
	r.DELETE("/api/ingress/:id", deleteIngressRule)

	r.GET("/api/status", getStatus)
	r.GET("/api/domains", listDomains)
	r.POST("/api/dns", createDNSRecord)
	r.DELETE("/api/dns/:zoneId/:recordId", deleteDNSRecord)

	r.Static("/assets", "./frontend/dist/assets")
	r.GET("/", func(c *gin.Context) {
		c.File("./frontend/dist/index.html")
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		c.File("./frontend/dist/index.html")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func initDB() error {
	dbPath := "tunnels.db"
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	var err error
	db, err = sql.Open("sqlite", dbPath+"?cache=shared")
	if err != nil {
		return err
	}

	db.Exec("PRAGMA user_version = 1")

	// Create table first; then ALTER adds columns missing on older DBs (ALTER before CREATE fails when table does not exist).
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tunnels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			uuid TEXT,
			account_id TEXT,
			zone_id TEXT,
			subdomain TEXT,
			domain TEXT,
			address TEXT,
			dns_record_id TEXT,
			tunnel_token TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'stopped',
			pid INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS ingress_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tunnel_id INTEGER NOT NULL,
			hostname TEXT NOT NULL,
			path TEXT,
			service TEXT NOT NULL,
			protocol TEXT DEFAULT 'http',
			FOREIGN KEY(tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tunnel_id INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			level TEXT,
			message TEXT,
			FOREIGN KEY(tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		return err
	}
	if _, e := db.Exec("ALTER TABLE tunnels ADD COLUMN IF NOT EXISTS address TEXT"); e != nil {
		_, _ = db.Exec("ALTER TABLE tunnels ADD COLUMN address TEXT")
	}
	if _, e := db.Exec("ALTER TABLE tunnels ADD COLUMN IF NOT EXISTS tunnel_token TEXT"); e != nil {
		_, _ = db.Exec("ALTER TABLE tunnels ADD COLUMN tunnel_token TEXT")
	}
	return nil
}

func listTunnels(c *gin.Context) {
	rows, err := db.Query("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, COALESCE(address, ''), created_at, status, pid FROM tunnels")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tunnels []Tunnel
	for rows.Next() {
		var t Tunnel
		rows.Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.CreatedAt, &t.Status, &t.PID)
		tunnels = append(tunnels, t)
	}
	c.JSON(http.StatusOK, tunnels)
}

func createTunnel(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		AccountID string `json:"account_id"`
		ZoneID    string `json:"zone_id"`
		// Domain is the zone apex hostname (e.g. example.com), not the Cloudflare zone UUID.
		Domain    string `json:"domain"`
		Subdomain string `json:"subdomain"`
		Address   string `json:"address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AccountID == "" {
		req.AccountID = cfg.AccountID
	}

	var tunnelUUID, tunnelToken string

	apex, fetched, apexErr := resolveZoneApex(req.ZoneID, req.Domain)
	if apexErr != nil {
		log.Printf("[DNS] resolve apex at create: %v", apexErr)
	}
	if apex != "" {
		req.Domain = apex
	} else if isLikelyLegacyCorruptDomain(req.Domain) {
		req.Domain = ""
	}

	if req.ZoneID != "" && req.Subdomain != "" && apex != "" && cfg.APIToken != "" && apexErr == nil {
		if req.AccountID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "CF_ACCOUNT_ID is required to register a tunnel with Cloudflare for DNS"})
			return
		}
		var err error
		tunnelUUID, tunnelToken, err = createRemoteCFDTunnel(req.AccountID, req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare tunnel registration failed: " + err.Error()})
			return
		}
	}

	result, err := db.Exec("INSERT INTO tunnels (name, account_id, zone_id, subdomain, domain, address, uuid, tunnel_token, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'stopped')",
		req.Name, req.AccountID, req.ZoneID, req.Subdomain, req.Domain, req.Address, tunnelUUID, tunnelToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	logTunnel(id, "info", "Tunnel created")
	if fetched && apex != "" {
		logTunnel(id, "info", "Resolved zone apex from Cloudflare API: "+apex)
	}

	if tunnelUUID != "" {
		applyTunnelDNS(int(id), req.ZoneID, req.Subdomain, apex, tunnelUUID, "")
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

func getTunnel(c *gin.Context) {
	id := c.Param("id")
	var t Tunnel
	err := db.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, address, created_at, status, pid FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.CreatedAt, &t.Status, &t.PID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func deleteTunnel(c *gin.Context) {
	id := c.Param("id")

	var t struct {
		UUID        string `json:"uuid"`
		DNSRecordID string `json:"dns_record_id"`
		ZoneID      string `json:"zone_id"`
		AccountID   string `json:"account_id"`
		Subdomain   string `json:"subdomain"`
		Domain      string `json:"domain"`
	}
	err := db.QueryRow("SELECT COALESCE(uuid, ''), COALESCE(dns_record_id, ''), COALESCE(zone_id, ''), COALESCE(account_id, ''), COALESCE(subdomain, ''), COALESCE(domain, '') FROM tunnels WHERE id = ?", id).
		Scan(&t.UUID, &t.DNSRecordID, &t.ZoneID, &t.AccountID, &t.Subdomain, &t.Domain)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}

	stopTunnelProcess(id)

	var warnings []string

	// DNS cleanup: prefer stored record ID, then fallback to lookup by name.
	if t.ZoneID != "" && cfg.APIToken != "" {
		recordID := strings.TrimSpace(t.DNSRecordID)
		if recordID == "" && strings.TrimSpace(t.Subdomain) != "" && strings.TrimSpace(t.Domain) != "" {
			fqdn := strings.TrimSpace(t.Subdomain) + "." + strings.TrimSpace(t.Domain)
			lookedUpID, lookErr := findCNAMERecordID(t.ZoneID, fqdn)
			if lookErr != nil {
				warnings = append(warnings, "DNS lookup before delete failed: "+lookErr.Error())
			}
			recordID = lookedUpID
		}
		if recordID != "" {
			log.Printf("[tunnel] Deleting DNS record %s from zone %s", recordID, t.ZoneID)
			if err := deleteDNSRecordByID(t.ZoneID, recordID); err != nil {
				warnings = append(warnings, "DNS delete failed: "+err.Error())
			}
		}
	}

	if t.UUID != "" && cfg.APIToken != "" {
		accID := t.AccountID
		if accID == "" {
			accID = cfg.AccountID
		}
		if accID == "" {
			log.Printf("[tunnel] Cannot delete Cloudflare tunnel - no account_id stored and CF_ACCOUNT_ID not configured")
		} else {
			log.Printf("[tunnel] Deleting Cloudflare tunnel %s from account %s", t.UUID, accID)
			if err := deleteRemoteTunnelWithRetry(accID, t.UUID); err != nil {
				warnings = append(warnings, "Cloudflare tunnel delete failed: "+err.Error())
			}
		}
	}

	_, err = db.Exec("DELETE FROM tunnels WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	msg := "Tunnel deleted"
	if len(warnings) > 0 {
		msg = msg + " (with warnings)"
	}
	c.JSON(http.StatusOK, gin.H{"message": msg, "warnings": warnings})
}

func findCNAMERecordID(zoneID, fqdn string) (string, error) {
	if zoneID == "" || fqdn == "" || cfg.APIToken == "" {
		return "", nil
	}
	reqURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=CNAME&name=%s", zoneID, url.QueryEscape(fqdn))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("parse dns lookup response: %w", err)
	}
	if !out.Success {
		msg := string(body)
		if len(out.Errors) > 0 && strings.TrimSpace(out.Errors[0].Message) != "" {
			msg = out.Errors[0].Message
		}
		return "", fmt.Errorf(strings.TrimSpace(msg))
	}
	if len(out.Result) == 0 {
		return "", nil
	}
	return out.Result[0].ID, nil
}

func deleteDNSRecordByID(zoneID, recordID string) error {
	req, err := http.NewRequest("DELETE", "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records/"+recordID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("parse dns delete response: %w", err)
	}
	if !out.Success {
		msg := string(body)
		if len(out.Errors) > 0 && strings.TrimSpace(out.Errors[0].Message) != "" {
			msg = out.Errors[0].Message
		}
		return fmt.Errorf(strings.TrimSpace(msg))
	}
	log.Printf("[tunnel] DNS record deleted (HTTP %d)", resp.StatusCode)
	return nil
}

func deleteRemoteTunnelWithRetry(accountID, tunnelID string) error {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		req, err := http.NewRequest("DELETE", "https://api.cloudflare.com/client/v4/accounts/"+accountID+"/cfd_tunnel/"+tunnelID, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("[tunnel] Cloudflare tunnel delete response (attempt %d): HTTP %d, body: %s", attempt, resp.StatusCode, string(body))

			var out struct {
				Success bool `json:"success"`
				Errors  []struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(body, &out); err != nil {
				lastErr = fmt.Errorf("parse tunnel delete response: %w", err)
			} else if out.Success {
				return nil
			} else {
				msg := string(body)
				code := 0
				if len(out.Errors) > 0 {
					code = out.Errors[0].Code
					if strings.TrimSpace(out.Errors[0].Message) != "" {
						msg = out.Errors[0].Message
					}
				}
				lastErr = fmt.Errorf(strings.TrimSpace(msg))
				// 1022 = active tunnel connections still present; retry after short backoff.
				if code == 1022 && attempt < 5 {
					time.Sleep(time.Duration(attempt) * time.Second)
					continue
				}
				return lastErr
			}
		}
		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}

func startTunnel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tunnel ID"})
		return
	}

	var t Tunnel
	var dnsRecordID string
	var tunnelToken string
	err = db.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, address, status, pid, COALESCE(dns_record_id, ''), COALESCE(tunnel_token, '') FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.Status, &t.PID, &dnsRecordID, &tunnelToken)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}

	if t.Status == "running" && t.PID > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tunnel already running"})
		return
	}

	if t.UUID == "" {
		acc := t.AccountID
		if acc == "" {
			acc = cfg.AccountID
		}
		if acc != "" && cfg.APIToken != "" {
			uid, tok, err := createRemoteCFDTunnel(acc, t.Name)
			if err != nil {
				logTunnel(id, "error", "Cloudflare tunnel registration failed: "+err.Error())
				c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare tunnel registration failed: " + err.Error()})
				return
			}
			t.UUID = uid
			tunnelToken = tok
			db.Exec("UPDATE tunnels SET uuid = ?, tunnel_token = ? WHERE id = ?", t.UUID, tunnelToken, id)
			logTunnel(id, "info", "Registered tunnel with Cloudflare: "+t.UUID)
		} else {
			t.UUID = generateToken()
			db.Exec("UPDATE tunnels SET uuid = ? WHERE id = ?", t.UUID, id)
			logTunnel(id, "info", "Generated local UUID (not a Cloudflare tunnel — DNS to .cfargotunnel.com will not work): "+t.UUID)
		}
	}

	apex, fetched, apexErr := resolveZoneApex(t.ZoneID, t.Domain)
	if apexErr != nil {
		log.Printf("[DNS] Could not resolve zone apex for zone_id=%s: %v", t.ZoneID, apexErr)
		logTunnel(id, "error", "Could not resolve zone apex: "+apexErr.Error())
	} else if fetched && apex != "" {
		t.Domain = apex
		db.Exec("UPDATE tunnels SET domain = ? WHERE id = ?", apex, id)
		logTunnel(id, "info", "Resolved zone apex from Cloudflare API: "+apex)
	}

	log.Printf("[DNS] ZoneID=%s Subdomain=%s Apex=%s APIToken=%v", t.ZoneID, t.Subdomain, apex, cfg.APIToken != "")
	applyTunnelDNS(id, t.ZoneID, t.Subdomain, apex, t.UUID, dnsRecordID)

	ingressRules, _ := getIngressRulesForTunnel(strconv.Itoa(id))

	if t.Address != "" && len(ingressRules) == 0 {
		hostname := ""
		if t.Subdomain != "" && apex != "" {
			hostname = t.Subdomain + "." + apex
		}
		_, err := db.Exec("INSERT INTO ingress_rules (tunnel_id, hostname, path, service, protocol) VALUES (?, ?, ?, ?, ?)",
			id, hostname, "", t.Address, "http")
		if err != nil {
			logTunnel(id, "error", "Failed to create ingress rule: "+err.Error())
		} else {
			ingressRules, _ = getIngressRulesForTunnel(strconv.Itoa(id))
			logTunnel(id, "info", "Created ingress rule for: "+t.Address)
		}
	}

	if len(ingressRules) == 0 && t.Address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No address specified and no ingress rules configured"})
		return
	}

	acc := t.AccountID
	if acc == "" {
		acc = cfg.AccountID
	}
	publicHost := ""
	if strings.TrimSpace(t.Subdomain) != "" && strings.TrimSpace(apex) != "" {
		publicHost = strings.TrimSpace(t.Subdomain) + "." + strings.TrimSpace(apex)
	}
	if tunnelToken != "" {
		if err := pushTunnelIngress(acc, t.UUID, ingressRules, publicHost); err != nil {
			logTunnel(id, "error", "Failed to push tunnel config to Cloudflare: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to push tunnel config: " + err.Error()})
			return
		}
		logTunnel(id, "info", "Pushed ingress configuration to Cloudflare")
	}

	exeDir, _ := filepath.Abs(".")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	cloudflaredPath := filepath.Join(exeDir, binName)
	if _, err := os.Stat(cloudflaredPath); err != nil {
		cloudflaredPath = binName
		exeDir = "."
	}

	logTunnel(id, "info", fmt.Sprintf("Starting tunnel with: %s", cloudflaredPath))

	var cmd *exec.Cmd
	if tunnelToken != "" {
		cmd = exec.Command(cloudflaredPath, "tunnel", "run", "--token", tunnelToken)
	} else {
		configFile := generateConfig(t.Name, t.UUID, ingressRules)
		logTunnel(id, "info", "Generated config: "+configFile)
		cmd = exec.Command(cloudflaredPath, "tunnel", "--config", configFile, "run", t.UUID)
	}
	cmd.Dir = exeDir
	cmd.Stdout = &logWriter{id: idStr, level: "info"}
	cmd.Stderr = &logWriter{id: idStr, level: "error"}

	logTunnel(id, "info", "Calling cmd.Start()...")
	if err := cmd.Start(); err != nil {
		logTunnel(id, "error", fmt.Sprintf("Failed to start: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logTunnel(id, "info", "Tunnel process started")

	db.Exec("UPDATE tunnels SET status = 'running', pid = ? WHERE id = ?", cmd.Process.Pid, id)
	tunnelProcs.Store(idStr, cmd.Process)

	logTunnel(id, "info", fmt.Sprintf("Tunnel started (PID: %d)", cmd.Process.Pid))
	c.JSON(http.StatusOK, gin.H{"message": "Tunnel started", "pid": cmd.Process.Pid})
}

func stopTunnel(c *gin.Context) {
	id := c.Param("id")

	stopTunnelProcess(id)

	db.Exec("UPDATE tunnels SET status = 'stopped', pid = 0 WHERE id = ?", id)
	logTunnel(id, "info", "Tunnel stopped")

	c.JSON(http.StatusOK, gin.H{"message": "Tunnel stopped"})
}

func stopTunnelProcess(id string) {
	if p, ok := tunnelProcs.Load(id); ok {
		proc := p.(*os.Process)
		proc.Kill()
		tunnelProcs.Delete(id)
	}

	var pid int
	db.QueryRow("SELECT pid FROM tunnels WHERE id = ?", id).Scan(&pid)
	if pid > 0 {
		proc, _ := os.FindProcess(pid)
		if proc != nil {
			proc.Kill()
		}
		db.Exec("UPDATE tunnels SET status = 'stopped', pid = 0 WHERE id = ?", id)
	}
}

func getTunnelLogs(c *gin.Context) {
	id := c.Param("id")
	limit := c.DefaultQuery("limit", "100")

	rows, err := db.Query("SELECT id, tunnel_id, timestamp, level, message FROM logs WHERE tunnel_id = ? ORDER BY timestamp DESC LIMIT ?", id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	logs := make([]LogEntry, 0)
	for rows.Next() {
		var l LogEntry
		rows.Scan(&l.ID, &l.TunnelID, &l.Timestamp, &l.Level, &l.Message)
		logs = append(logs, l)
	}
	c.JSON(http.StatusOK, logs)
}

func getAllLogs(c *gin.Context) {
	limit := c.DefaultQuery("limit", "500")

	rows, err := db.Query("SELECT id, tunnel_id, timestamp, level, message FROM logs ORDER BY timestamp DESC LIMIT ?", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	logs := make([]LogEntry, 0)
	for rows.Next() {
		var l LogEntry
		rows.Scan(&l.ID, &l.TunnelID, &l.Timestamp, &l.Level, &l.Message)
		logs = append(logs, l)
	}
	c.JSON(http.StatusOK, logs)
}

func listIngressRules(c *gin.Context) {
	tunnelID := c.Query("tunnel_id")
	if tunnelID != "" {
		rows, err := db.Query("SELECT id, tunnel_id, hostname, path, service, protocol FROM ingress_rules WHERE tunnel_id = ?", tunnelID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var rules []IngressRule
		for rows.Next() {
			var r IngressRule
			rows.Scan(&r.ID, &r.TunnelID, &r.Hostname, &r.Path, &r.Service, &r.Protocol)
			rules = append(rules, r)
		}
		c.JSON(http.StatusOK, rules)
		return
	}

	rows, err := db.Query("SELECT id, tunnel_id, hostname, path, service, protocol FROM ingress_rules")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var rules []IngressRule
	for rows.Next() {
		var r IngressRule
		rows.Scan(&r.ID, &r.TunnelID, &r.Hostname, &r.Path, &r.Service, &r.Protocol)
		rules = append(rules, r)
	}
	c.JSON(http.StatusOK, rules)
}

func createIngressRule(c *gin.Context) {
	var r IngressRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := db.Exec("INSERT INTO ingress_rules (tunnel_id, hostname, path, service, protocol) VALUES (?, ?, ?, ?, ?)",
		r.TunnelID, r.Hostname, r.Path, r.Service, r.Protocol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func updateIngressRule(c *gin.Context) {
	id := c.Param("id")
	var r IngressRule
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := db.Exec("UPDATE ingress_rules SET hostname = ?, path = ?, service = ?, protocol = ? WHERE id = ?",
		r.Hostname, r.Path, r.Service, r.Protocol, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Rule updated"})
}

func deleteIngressRule(c *gin.Context) {
	id := c.Param("id")
	_, err := db.Exec("DELETE FROM ingress_rules WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted"})
}

func getStatus(c *gin.Context) {
	var total, running, stopped int
	db.QueryRow("SELECT COUNT(*), SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), SUM(CASE WHEN status = 'stopped' THEN 1 ELSE 0 END) FROM tunnels").Scan(&total, &running, &stopped)

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"running": running,
		"stopped": stopped,
	})
}

func getIngressRulesForTunnel(tunnelID string) ([]IngressRule, error) {
	rows, err := db.Query("SELECT id, tunnel_id, hostname, path, service, protocol FROM ingress_rules WHERE tunnel_id = ?", tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []IngressRule
	for rows.Next() {
		var r IngressRule
		rows.Scan(&r.ID, &r.TunnelID, &r.Hostname, &r.Path, &r.Service, &r.Protocol)
		rules = append(rules, r)
	}
	return rules, nil
}

func generateConfig(name, uuid string, rules []IngressRule) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("tunnelName: %s\ntunnelID: %s\n", name, uuid))
	buf.WriteString("ingress:\n")

	for _, r := range rules {
		buf.WriteString("  - hostname: " + r.Hostname + "\n")
		buf.WriteString("    service: " + originServiceURLForIngress(r.Service) + "\n")
		if r.Path != "" {
			buf.WriteString("    path: " + r.Path + "\n")
		}
	}

	configDir := filepath.Join(os.TempDir(), "cloudflared")
	os.MkdirAll(configDir, 0755)
	path := filepath.Join(configDir, name+".yml")
	os.WriteFile(path, []byte(buf.String()), 0644)
	return path
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// sanitizeTunnelNameForCF maps app tunnel names to Cloudflare's allowed tunnel name characters.
func sanitizeTunnelNameForCF(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-', r == '_', r == ' ', r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 90 {
		out = out[:90]
	}
	if out == "" {
		out = "tunnel"
	}
	return out
}

func tunnelNameSuffix() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// originServiceURLForIngress returns an origin-only service URL for Cloudflare tunnel ingress.
// Cloudflare rejects any path/query on the service URL ("eyeball request's path" error). Trailing slashes count.
//
// net/url.Parse mis-parses bare "host:port" (e.g. localhost:3000) as scheme "localhost", so we prepend http:// when there is no "://" and no usable Host.
func originServiceURLForIngress(service string) string {
	orig := strings.TrimSpace(service)
	if orig == "" {
		return orig
	}
	s := orig
	u, err := url.Parse(s)
	if err != nil {
		return orig
	}
	// Bare host:port without scheme — Go assigns a fake scheme; Host stays empty.
	if !strings.Contains(s, "://") && u.Host == "" {
		u2, err2 := url.Parse("http://" + s)
		if err2 == nil && u2.Host != "" {
			u = u2
		}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "unix" {
		return orig
	}
	if scheme != "http" && scheme != "https" && scheme != "tcp" && scheme != "udp" {
		return orig
	}
	if u.Host == "" {
		return orig
	}
	var clean string
	switch scheme {
	case "http", "https":
		clean = (&url.URL{Scheme: u.Scheme, User: u.User, Host: u.Host}).String()
	case "tcp", "udp":
		clean = (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	default:
		return orig
	}
	if clean != orig {
		log.Printf("[tunnel] normalized service URL %q -> %q (Cloudflare ingress requires origin root only)", orig, clean)
	}
	return clean
}

func normalizeIngressHostname(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimSuffix(h, ".")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSpace(h)
}

func logTunnel(tunnelID interface{}, level, msg string) {
	_, err := db.Exec("INSERT INTO logs (tunnel_id, level, message, timestamp) VALUES (?, ?, ?, datetime('now'))", tunnelID, level, msg)
	if err != nil {
		log.Printf("[LOG ERROR] Failed to insert log: %v", err)
	}
}

type logWriter struct {
	id    interface{}
	level string
}

func (w logWriter) Write(p []byte) (int, error) {
	logTunnel(w.id, w.level, string(p))
	return len(p), nil
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// isLikelyLegacyCorruptDomain detects values stored by the old bug (subdomain + "." + zone UUID).
func isLikelyLegacyCorruptDomain(domain string) bool {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return false
	}
	i := strings.LastIndex(domain, ".")
	if i <= 0 {
		return false
	}
	tail := domain[i+1:]
	if len(tail) != 32 {
		return false
	}
	for _, c := range tail {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func fetchZoneName(zoneID string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/zones/"+zoneID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if !result.Success && len(result.Errors) > 0 {
		return "", fmt.Errorf("%s", result.Errors[0].Message)
	}
	if result.Result.Name == "" {
		return "", fmt.Errorf("empty zone name")
	}
	return result.Result.Name, nil
}

// createRemoteCFDTunnel registers a named tunnel in Cloudflare so DNS CNAME targets like {id}.cfargotunnel.com are valid.
// On HTTP 409 (name already taken), retries with a suffixed name so a leftover tunnel from a failed run does not block creates.
func createRemoteCFDTunnel(accountID, tunnelName string) (tunnelID, tunnelToken string, err error) {
	if accountID == "" {
		return "", "", fmt.Errorf("account ID is empty")
	}
	if cfg.APIToken == "" {
		return "", "", fmt.Errorf("API token is empty")
	}
	base := sanitizeTunnelNameForCF(tunnelName)
	for attempt := 0; attempt < 8; attempt++ {
		cfName := base
		if attempt > 0 {
			cfName = base + "-" + tunnelNameSuffix()
			if len(cfName) > 100 {
				cfName = cfName[:100]
			}
		}
		body, err := json.Marshal(map[string]string{
			"name":       cfName,
			"config_src": "cloudflare",
		})
		if err != nil {
			return "", "", err
		}
		url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel", accountID)
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var out struct {
			Success bool `json:"success"`
			Result  struct {
				ID    string `json:"id"`
				Token string `json:"token"`
			} `json:"result"`
			Errors []struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return "", "", fmt.Errorf("parse response: %w", err)
		}
		if out.Success && len(out.Errors) == 0 && out.Result.ID != "" && out.Result.Token != "" {
			if attempt > 0 {
				log.Printf("[tunnel] registered in Cloudflare as %q (name %q was already taken)", cfName, base)
			}
			return out.Result.ID, out.Result.Token, nil
		}
		msg := "unknown error"
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		if resp.StatusCode == 409 {
			log.Printf("[tunnel] Cloudflare 409 for name %q: %s — retrying with a new suffix", cfName, msg)
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 ||
			strings.EqualFold(msg, "Authentication error") ||
			strings.Contains(strings.ToLower(msg), "auth") {
			accHint := strings.TrimSpace(validateAccountIDForTunnelAPI(accountID))
			if accHint != "" {
				accHint = " " + accHint
			}
			return "", "", fmt.Errorf("%s (HTTP %d).%s Also ensure the token has Cloudflare One Connector: cloudflared — Write (tunnel registration is not allowed with Zone DNS–only tokens).",
				msg, resp.StatusCode, accHint)
		}
		return "", "", fmt.Errorf("%s (HTTP %d)", msg, resp.StatusCode)
	}
	return "", "", fmt.Errorf("could not register tunnel after retries: Cloudflare keeps returning 409 (name conflict). Delete the old tunnel in Zero Trust → Networks → Tunnels, or pick another tunnel name in this app")
}

// pushTunnelIngress uploads ingress rules for a remotely managed tunnel (used with tunnel run --token).
// publicHost is the FQDN for this tunnel (e.g. app.example.com); used when a stored rule has no hostname.
// Cloudflare rejects multiple ingress rows without hostname (only the final catch-all may omit it).
func pushTunnelIngress(accountID, tunnelID string, rules []IngressRule, publicHost string) error {
	if accountID == "" || tunnelID == "" {
		return fmt.Errorf("account ID and tunnel ID are required")
	}
	publicHost = normalizeIngressHostname(publicHost)
	type cfIngress struct {
		Hostname string `json:"hostname,omitempty"`
		Path     string `json:"path,omitempty"`
		Service  string `json:"service"`
	}
	// Deduplicate by (hostname, path): Cloudflare rejects overlapping rules with the same eyeball match.
	seen := make(map[string]int)
	ingress := make([]cfIngress, 0, len(rules)+1)
	for _, r := range rules {
		host := strings.TrimSpace(r.Hostname)
		if host == "" {
			host = publicHost
		}
		host = normalizeIngressHostname(host)
		if host == "" {
			return fmt.Errorf("ingress needs a public hostname: set subdomain + domain on the tunnel, or give each ingress rule a hostname (Cloudflare does not allow multiple hostname-less rules)")
		}
		pathKey := strings.TrimSpace(r.Path)
		if pathKey == "/" {
			pathKey = ""
		}
		key := host + "\x00" + pathKey
		ing := cfIngress{
			Hostname: host,
			Service:  originServiceURLForIngress(r.Service),
		}
		if pathKey != "" {
			ing.Path = pathKey
		}
		if i, ok := seen[key]; ok {
			ingress[i] = ing
			continue
		}
		seen[key] = len(ingress)
		ingress = append(ingress, ing)
	}
	ingress = append(ingress, cfIngress{Service: "http_status:404"})
	payload := map[string]interface{}{
		"config": map[string]interface{}{
			"ingress": ingress,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel/%s/configurations", accountID, tunnelID)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Messages []struct {
			Message string `json:"message"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !out.Success || len(out.Errors) > 0 {
		msg := string(respBody)
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		for _, m := range out.Messages {
			if strings.TrimSpace(m.Message) != "" {
				msg = msg + " | " + m.Message
			}
		}
		log.Printf("[tunnel] push ingress rejected (HTTP %d): %s | sent: %s", resp.StatusCode, msg, string(body))
		return fmt.Errorf("%s", strings.TrimSpace(msg))
	}
	return nil
}

// resolveZoneApex returns the apex hostname (e.g. example.com). fetchedFromAPI is true when loaded via GET /zones/{id}.
func resolveZoneApex(zoneID, domainStored string) (apex string, fetchedFromAPI bool, err error) {
	apex = strings.TrimSpace(domainStored)
	if isLikelyLegacyCorruptDomain(apex) {
		apex = ""
	}
	if apex != "" {
		return apex, false, nil
	}
	if zoneID == "" || cfg.APIToken == "" {
		return "", false, nil
	}
	name, err := fetchZoneName(zoneID)
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

// applyTunnelDNS creates the Cloudflare CNAME for this tunnel if missing.
func applyTunnelDNS(id int, zoneID, subdomain, apex, tunnelUUID, existingDNSID string) {
	if existingDNSID != "" {
		logTunnel(id, "info", "DNS record already present, skipping creation")
		return
	}
	if zoneID == "" || subdomain == "" || apex == "" || cfg.APIToken == "" || tunnelUUID == "" {
		logTunnel(id, "info", "Skipping DNS - need zone_id, subdomain, apex, tunnel UUID, and API token")
		return
	}
	fullDomain := subdomain + "." + apex
	log.Printf("[DNS] Creating CNAME: %s -> %s.cfargotunnel.com", fullDomain, tunnelUUID)
	recordID, err := createCNAME(zoneID, fullDomain, tunnelUUID+".cfargotunnel.com")
	if err != nil {
		log.Printf("[DNS] ERROR: %v", err)
		logTunnel(id, "error", "DNS CNAME failed: "+err.Error())
		return
	}
	if recordID != "" {
		db.Exec("UPDATE tunnels SET dns_record_id = ? WHERE id = ?", recordID, id)
		logTunnel(id, "info", "DNS CNAME record created: "+fullDomain+" -> "+tunnelUUID+".cfargotunnel.com")
		log.Printf("[DNS] Created record ID: %s", recordID)
	} else {
		log.Printf("[DNS] Empty recordID returned")
		logTunnel(id, "error", "DNS CNAME returned empty recordID")
	}
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func listDomains(c *gin.Context) {
	if cfg.APIToken == "" || cfg.AccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token or account ID not configured"})
		return
	}

	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")

	req, _ := http.NewRequest("GET", "https://api.cloudflare.com/client/v4/zones?page="+page+"&per_page="+perPage, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Cloudflare API response status: %d", resp.StatusCode)

	var result struct {
		Success  bool   `json:"success"`
		Errors   []struct{ Message string } `json:"errors"`
		Result   []Zone `json:"result"`
		ResultInfo struct {
			TotalCount int `json:"total_count"`
		} `json:"result_info"`
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	json.Unmarshal(body, &result)

	if !result.Success && len(result.Errors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Errors[0].Message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"domains":  result.Result,
		"total":    result.ResultInfo.TotalCount,
		"page":     result.Page,
		"per_page": result.PerPage,
	})
}

func createDNSRecord(c *gin.Context) {
	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}

	var req struct {
		ZoneID  string `json:"zone_id" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
		Type    string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" {
		req.Type = "CNAME"
	}

	body, _ := json.Marshal(map[string]string{
		"type":    req.Type,
		"name":    req.Name,
		"content": req.Content,
	})

	httpReq, _ := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/zones/"+req.ZoneID+"/dns_records", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Result DNSRecord `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	c.JSON(http.StatusCreated, result.Result)
}

func deleteDNSRecord(c *gin.Context) {
	zoneID := c.Param("zoneId")
	recordID := c.Param("recordId")

	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}

	httpReq, _ := http.NewRequest("DELETE", "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records/"+recordID, nil)
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIToken)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	c.JSON(http.StatusOK, gin.H{"message": "DNS record deleted"})
}

func createCNAME(zoneID, name, content string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": content,
		"proxied": true,
	})

	log.Printf("[DNS API] POST to zones/%s/dns_records", zoneID)
	log.Printf("[DNS API] Body: %s", string(body))

	httpReq, _ := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[DNS API] Request failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[DNS API] Response status: %d body: %s", resp.StatusCode, string(respBody))

	var result struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	json.Unmarshal(respBody, &result)

	if len(result.Errors) > 0 {
		log.Printf("[DNS API] Errors: %+v", result.Errors)
		return "", fmt.Errorf(result.Errors[0].Message)
	}

	return result.Result.ID, nil
}