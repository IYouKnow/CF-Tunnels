package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cf-tunnel-manager/backend/internal/apps"
	"github.com/cf-tunnel-manager/backend/internal/cloudflare"
	dnsservice "github.com/cf-tunnel-manager/backend/internal/dns"
	"github.com/cf-tunnel-manager/backend/internal/tunnels"
	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
	"github.com/spf13/viper"
)

var AppVersion = "dev"

type Config struct {
	APIToken      string `mapstructure:"CF_API_TOKEN" env:"CF_API_TOKEN"`
	AccountID     string `mapstructure:"CF_ACCOUNT_ID" env:"CF_ACCOUNT_ID"`
	AdminUser     string `mapstructure:"ADMIN_USER" env:"ADMIN_USER"`
	AdminPass     string `mapstructure:"ADMIN_PASSWORD" env:"ADMIN_PASSWORD"`
	ListenPort    int    `mapstructure:"LISTEN_PORT" env:"LISTEN_PORT"`
	SessionSecret string `mapstructure:"SESSION_SECRET" env:"SESSION_SECRET"`
}

type Tunnel struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	UUID      string    `json:"uuid"`
	AccountID string    `json:"account_id"`
	ZoneID    string    `json:"zone_id,omitempty"`
	Subdomain string    `json:"subdomain,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	Address   string    `json:"address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
	PID       int       `json:"pid,omitempty"`
}

type IngressRule struct {
	ID       int    `json:"id"`
	TunnelID int    `json:"tunnel_id"`
	Hostname string `json:"hostname"`
	Path     string `json:"path,omitempty"`
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

type LogEvent struct {
	ID        int64       `json:"id"`
	TunnelID  interface{} `json:"tunnel_id"`
	Timestamp string      `json:"timestamp"`
	Level     string      `json:"level"`
	Message   string      `json:"message"`
}

type LogBroadcaster struct {
	mu   sync.RWMutex
	subs map[chan LogEvent]struct{}
}

func (lb *LogBroadcaster) Subscribe() chan LogEvent {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	ch := make(chan LogEvent, 128)
	lb.subs[ch] = struct{}{}
	return ch
}

func (lb *LogBroadcaster) Unsubscribe(ch chan LogEvent) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	delete(lb.subs, ch)
	close(ch)
}

func (lb *LogBroadcaster) Publish(event LogEvent) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	for ch := range lb.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

var (
	db             *sql.DB
	cfg            Config
	appSvc         *apps.Service
	cfClient       *cloudflare.Client
	dnsSvc         *dnsservice.Service
	tunnelSvc      *tunnels.Service
	logBroadcaster = &LogBroadcaster{subs: make(map[chan LogEvent]struct{})}
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

var sessionKey []byte

func initSessionKey(dataDir string) {
	s := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if s != "" {
		sessionKey = []byte(s)
		log.Println("[AUTH] Using SESSION_SECRET from environment")
		return
	}

	keyPath := filepath.Join(dataDir, "session.key")
	existing, err := os.ReadFile(keyPath)
	if err == nil && len(existing) >= 32 {
		sessionKey = existing[:32]
		log.Println("[AUTH] Loaded session key from", keyPath)
		return
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatalf("[AUTH] Failed to generate session key: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0700); err == nil {
		os.WriteFile(keyPath, key, 0600)
	}
	sessionKey = key
	log.Println("[AUTH] Generated new session key at", keyPath)
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
	mac := hmac.New(sha256.New, sessionKey)
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
	mac := hmac.New(sha256.New, sessionKey)
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

type loginRateLimiter struct {
	mu      sync.Mutex
	attempts map[string][]time.Time
}

func (rl *loginRateLimiter) allow(ip string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	window := 5 * time.Minute
	maxAttempts := 5

	entries := rl.attempts[ip]
	var recent []time.Time
	for _, t := range entries {
		if now.Sub(t) < window {
			recent = append(recent, t)
		}
	}

	if len(recent) >= maxAttempts {
		retryAfter := window - now.Sub(recent[0])
		return false, retryAfter
	}

	recent = append(recent, now)
	rl.attempts[ip] = recent
	return true, 0
}

var loginLimiter = &loginRateLimiter{attempts: make(map[string][]time.Time)}

func postLogin(c *gin.Context) {
	ip := c.ClientIP()
	allowed, retryAfter := loginLimiter.allow(ip)
	if !allowed {
		secs := int(retryAfter.Seconds()) + 1
		c.Header("Retry-After", strconv.Itoa(secs))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": fmt.Sprintf("Too many login attempts. Try again in %d seconds.", secs)})
		return
	}

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

func getAuthConfigStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"credentialsConfigured": cfg.AdminUser != "" && cfg.AdminPass != "",
	})
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

func appTokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			return
		}
		token := strings.TrimSpace(authHeader[len("Bearer "):])
		authenticated, err := appSvc.AuthenticateToken(c.Request.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, apps.ErrMissingToken), errors.Is(err, apps.ErrTokenInvalid):
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid app token"})
			case errors.Is(err, apps.ErrTokenExpired):
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "App token expired"})
			case errors.Is(err, apps.ErrTokenRevoked):
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "App token revoked"})
			default:
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Could not authenticate app token"})
			}
			return
		}
		c.Set("app", authenticated.App)
		c.Set("app_scopes", authenticated.Token.Scopes)
		c.Next()
	}
}

func requireAppScope(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, exists := c.Get("app_scopes")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Missing app scopes"})
			return
		}
		scopes, ok := raw.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Invalid app scopes"})
			return
		}
		if err := apps.RequireScope(scopes, required); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.Next()
	}
}

func normalizeCFConfig(c *Config) {
	c.APIToken = strings.TrimSpace(strings.Trim(c.APIToken, `"'`))
	c.AccountID = strings.TrimSpace(strings.Trim(c.AccountID, `"'`))
}

func validateAccountIDForTunnelAPI(accountID string) string {
	if cfClient == nil {
		return ""
	}
	if err := cfClient.ValidateAccountID(context.Background(), accountID); err != nil {
		return " " + err.Error()
	}
	return ""
}

func dataDir() string {
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data"
	}
	return "."
}

func webRootDir() string {
	if _, err := os.Stat("/app/share/web"); err == nil {
		return "/app/share/web"
	}
	return defaultWebRootDir()
}

// defaultWebRootDir picks the Vite out dir for local runs (repo root vs backend/ cwd).
func defaultWebRootDir() string {
	candidates := []string{
		filepath.Join("frontend", "dist"),
		filepath.Join("..", "frontend", "dist"),
	}
	for _, p := range candidates {
		st, err := os.Stat(filepath.Join(p, "index.html"))
		if err == nil && !st.IsDir() {
			return p
		}
	}
	return filepath.Join("..", "frontend", "dist")
}

func main() {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	viper.Unmarshal(&cfg)
	normalizeCFConfig(&cfg)

	// Prefer OS env (Docker/CasaOS): viper.Unmarshal often does not bind env into the struct.
	if v := strings.TrimSpace(os.Getenv("CF_API_TOKEN")); v != "" {
		cfg.APIToken = v
	}
	if v := strings.TrimSpace(os.Getenv("CF_ACCOUNT_ID")); v != "" {
		cfg.AccountID = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_USER")); v != "" {
		cfg.AdminUser = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")); v != "" {
		cfg.AdminPass = v
	}
	if v := strings.TrimSpace(os.Getenv("SESSION_SECRET")); v != "" {
		cfg.SessionSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("LISTEN_PORT")); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.ListenPort = p
		}
	}
	normalizeCFConfig(&cfg)
	cfClient = cloudflare.NewClient(cfg.APIToken, cfg.AccountID)

	if cfg.AdminUser == "" {
		log.Println("[AUTH] WARNING: ADMIN_USER is not set. Login will be unavailable until ADMIN_USER is set in .env or environment.")
	}
	if cfg.AdminPass == "" {
		log.Println("[AUTH] WARNING: ADMIN_PASSWORD is not set. Login will be unavailable until ADMIN_PASSWORD is set in .env or environment.")
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 38427
	}
	initSessionKey(dataDir())
	cfClient = cloudflare.NewClient(cfg.APIToken, cfg.AccountID)

	if err := initDB(); err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	// Route standard library logs to a file in the data directory.
	logFile := filepath.Join(dataDir(), "app.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(f)
		defer f.Close()
	} else {
		log.SetOutput(os.Stderr)
	}
	appSvc = apps.NewService(db)
	dnsSvc = dnsservice.NewService(cfClient)
	tunnelSvc = tunnels.NewService(db, cfClient, cfg.AccountID, cfg.APIToken != "", &tunnelProcs, logTunnel, newTunnelLogWriter)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cspMiddleware())
	r.Use(corsMiddleware())

	r.Use(authMiddleware())

	r.GET("/api/auth/config-status", getAuthConfigStatus)
	r.POST("/api/login", postLogin)
	r.POST("/api/logout", postLogout)
	r.GET("/api/auth/me", getAuthMe)

	r.GET("/api/tunnels", listTunnels)
	r.POST("/api/tunnels", createTunnel)
	r.GET("/api/tunnels/:id", getTunnel)
	r.PUT("/api/tunnels/:id", updateTunnel)
	r.DELETE("/api/tunnels/:id", deleteTunnel)
	r.POST("/api/tunnels/sync", syncTunnels)
	r.POST("/api/tunnels/:id/start", startTunnel)
	r.POST("/api/tunnels/:id/stop", stopTunnel)
	r.GET("/api/tunnels/:id/logs", getTunnelLogs)
	r.GET("/api/logs", getAllLogs)
	r.GET("/api/logs/stream", streamLogs)

	r.GET("/api/ingress", listIngressRules)
	r.POST("/api/ingress", createIngressRule)
	r.PUT("/api/ingress/:id", updateIngressRule)
	r.DELETE("/api/ingress/:id", deleteIngressRule)

	r.GET("/api/status", getStatus)
	r.GET("/api/domains", listDomains)
	r.GET("/api/dns/records", listDNSRecords)
	r.POST("/api/dns", createDNSRecord)
	r.PUT("/api/dns/:zoneId/:recordId", updateDNSRecord)
	r.DELETE("/api/dns/:zoneId/:recordId", deleteDNSRecord)
	r.GET("/api/cloudflared/version", getCloudflaredVersion)
	r.GET("/api/cloudflared/check-update", getCloudflaredUpdate)
	r.POST("/api/cloudflared/update", updateCloudflared)

	r.GET("/api/app/version", getAppVersion)
	r.GET("/api/app/check-update", getAppUpdate)
	r.POST("/api/app/update", updateApp)

	r.GET("/api/apps", listApps)
	r.POST("/api/apps", createApp)
	r.GET("/api/apps/:id", getApp)
	r.DELETE("/api/apps/:id", deleteApp)
	r.GET("/api/apps/:id/tokens", listAppTokens)
	r.POST("/api/apps/:id/tokens", createAppToken)
	r.DELETE("/api/apps/:id/tokens/:tokenId", revokeAppToken)
	r.DELETE("/api/apps/:id/tokens/:tokenId/purge", deleteAppToken)

	v1 := gin.New()
	v1.Use(gin.Recovery())
	v1.Use(cspMiddleware())
	v1.Use(corsMiddleware())
	// TODO: Extend this group with app-token-protected central API routes as tunnel/DNS
	// orchestration is exposed to other internal apps.
	v1.GET("/api/v1/me", appTokenAuthMiddleware(), requireAppScope("resources:read"), getAppMe)
	v1.POST("/api/v1/dns/ensure", appTokenAuthMiddleware(), ensureAppDNSRecord)
	v1.GET("/api/v1/dns/:hostname", appTokenAuthMiddleware(), requireAppScope("dns:read"), getAppDNSRecord)
	v1.PATCH("/api/v1/dns/:hostname", appTokenAuthMiddleware(), requireAppScope("dns:update"), patchAppDNSRecord)

	webRoot := filepath.Clean(webRootDir())
	assetsFS := filepath.Join(webRoot, "assets")
	indexHTML := filepath.Join(webRoot, "index.html")
	r.Static("/assets", assetsFS)
	r.GET("/", func(c *gin.Context) {
		c.File(indexHTML)
	})

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		c.File(indexHTML)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: joinRouters(r, v1),
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

	tunnelSvc.StopAll()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func corsMiddleware() gin.HandlerFunc {
	allowedOrigins := map[string]bool{
		"http://localhost:8080": true,
		"http://127.0.0.1:8080": true,
		"http://localhost:38427": true,
		"http://127.0.0.1:38427": true,
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if allowedOrigins[origin] {
				c.Header("Access-Control-Allow-Origin", origin)
			}
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func cspMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'none'")
		c.Next()
	}
}

func joinRouters(primary http.Handler, v1 http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			v1.ServeHTTP(w, r)
			return
		}
		primary.ServeHTTP(w, r)
	})
}

func initDB() error {
	dir := filepath.Clean(dataDir())
	dbPath := filepath.Join(dir, "tunnels.db")

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// One-time move from legacy cwd ./tunnels.db (e.g. old Docker layout /app/tunnels.db).
	if _, err := os.Stat(dbPath); os.IsNotExist(err) && dir != "." {
		legacy := filepath.Join(".", "tunnels.db")
		if st, err := os.Stat(legacy); err == nil && !st.IsDir() {
			if err := os.Rename(legacy, dbPath); err != nil {
				return fmt.Errorf("migrate legacy tunnels.db: %w", err)
			}
		}
	}

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
		CREATE TABLE IF NOT EXISTS apps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS app_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			scopes TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			last_used_at DATETIME,
			revoked_at DATETIME,
			FOREIGN KEY(app_id) REFERENCES apps(id) ON DELETE CASCADE
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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}
	q := strings.TrimSpace(c.Query("q"))

	where := ""
	args := []interface{}{}
	if q != "" {
		like := "%" + q + "%"
		where = "WHERE name LIKE ? OR subdomain LIKE ? OR uuid LIKE ?"
		args = append(args, like, like, like)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM tunnels " + where
	db.QueryRow(countQuery, args...).Scan(&total)

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)
	rows, err := db.Query("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, COALESCE(address, ''), created_at, status, pid FROM tunnels "+where+" ORDER BY name LIMIT ? OFFSET ?", args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	tunnels := make([]Tunnel, 0)
	for rows.Next() {
		var t Tunnel
		rows.Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.CreatedAt, &t.Status, &t.PID)
		tunnels = append(tunnels, t)
	}
	c.JSON(http.StatusOK, gin.H{
		"tunnels":  tunnels,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
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
	result, err := tunnelSvc.CreateTunnel(c.Request.Context(), tunnels.CreateTunnelInput{
		Name:      req.Name,
		AccountID: req.AccountID,
		ZoneID:    req.ZoneID,
		Domain:    req.Domain,
		Subdomain: req.Subdomain,
		Address:   req.Address,
	})
	if err != nil {
		var badReq *tunnels.BadRequestError
		if errors.As(err, &badReq) {
			c.JSON(http.StatusBadRequest, gin.H{"error": badReq.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": result.ID, "name": result.Name})
}

func updateTunnel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tunnel ID"})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := tunnelSvc.UpdateTunnelName(c.Request.Context(), id, req.Name); err != nil {
		if errors.Is(err, tunnels.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Tunnel renamed"})
}

func syncTunnels(c *gin.Context) {
	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}
	imported, updated, err := tunnelSvc.SyncTunnels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "updated": updated})
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
	result, err := tunnelSvc.DeleteTunnel(c.Request.Context(), c.Param("id"))
	if errors.Is(err, tunnels.ErrTunnelNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": result.Message, "warnings": result.Warnings})
}

func startTunnel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tunnel ID"})
		return
	}

	result, err := tunnelSvc.StartTunnel(c.Request.Context(), id)
	if errors.Is(err, tunnels.ErrTunnelNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	var badReq *tunnels.BadRequestError
	if errors.As(err, &badReq) {
		c.JSON(http.StatusBadRequest, gin.H{"error": badReq.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tunnel started", "pid": result.PID})
}

func stopTunnel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tunnel ID"})
		return
	}

	err = tunnelSvc.StopTunnel(c.Request.Context(), strconv.Itoa(id))
	if errors.Is(err, tunnels.ErrTunnelNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tunnel stopped"})
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

func streamLogs(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	ch := logBroadcaster.Subscribe()
	defer logBroadcaster.Unsubscribe(ch)

	tunnelID := c.Query("tunnel_id")

	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-ch:
			if !ok {
				return false
			}
			if tunnelID != "" && fmt.Sprintf("%v", event.TunnelID) != tunnelID {
				return true
			}
			data, err := json.Marshal(event)
			if err != nil {
				return true
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
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

	if r.TunnelID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel_id is required"})
		return
	}
	if strings.TrimSpace(r.Hostname) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hostname is required"})
		return
	}
	if strings.TrimSpace(r.Service) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service is required"})
		return
	}

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM tunnels WHERE id = ?)", r.TunnelID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel not found"})
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
		"total":   total,
		"running": running,
		"stopped": stopped,
	})
}

func logTunnel(tunnelID interface{}, level, msg string) {
	result, err := db.Exec("INSERT INTO logs (tunnel_id, level, message, timestamp) VALUES (?, ?, ?, datetime('now'))", tunnelID, level, msg)
	if err != nil {
		log.Printf("[LOG ERROR] Failed to insert log: %v", err)
		return
	}
	id, _ := result.LastInsertId()
	logBroadcaster.Publish(LogEvent{
		ID:        id,
		TunnelID:  tunnelID,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     level,
		Message:   msg,
	})
}

type logWriter struct {
	id    interface{}
	level string
}

func newTunnelLogWriter(id string, level string) io.Writer {
	return logWriter{id: id, level: level}
}

func (w logWriter) Write(p []byte) (int, error) {
	logTunnel(w.id, w.level, string(p))
	return len(p), nil
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid " + name})
		return 0, false
	}
	return id, true
}

func listApps(c *gin.Context) {
	items, err := appSvc.ListApps(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func createApp(c *gin.Context) {
	var req apps.CreateAppInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := appSvc.CreateApp(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, apps.ErrInvalidSlug):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Slug must use lowercase letters, numbers, and hyphens only"})
		case errors.Is(err, apps.ErrDuplicateSlug):
			c.JSON(http.StatusBadRequest, gin.H{"error": "App slug already exists"})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, item)
}

func getApp(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	item, err := appSvc.GetApp(c.Request.Context(), id)
	if errors.Is(err, apps.ErrAppNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func deleteApp(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	err := appSvc.DeleteApp(c.Request.Context(), id)
	switch {
	case errors.Is(err, apps.ErrAppNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
	case errors.Is(err, apps.ErrAppHasTokens):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Revoke app tokens before deleting this app"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"message": "App deleted"})
	}
}

func listAppTokens(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	items, err := appSvc.ListTokens(c.Request.Context(), id)
	if errors.Is(err, apps.ErrAppNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func createAppToken(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req apps.CreateAppTokenInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := appSvc.CreateToken(c.Request.Context(), id, req)
	if errors.Is(err, apps.ErrAppNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func revokeAppToken(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	tokenID, ok := parseIDParam(c, "tokenId")
	if !ok {
		return
	}
	err := appSvc.RevokeToken(c.Request.Context(), id, tokenID)
	switch {
	case errors.Is(err, apps.ErrAppNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
	case errors.Is(err, apps.ErrTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"message": "Token revoked"})
	}
}

func deleteAppToken(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	tokenID, ok := parseIDParam(c, "tokenId")
	if !ok {
		return
	}
	err := appSvc.DeleteToken(c.Request.Context(), id, tokenID)
	switch {
	case errors.Is(err, apps.ErrAppNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
	case errors.Is(err, apps.ErrTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"message": "Token permanently deleted"})
	}
}

func getCloudflaredVersion(c *gin.Context) {
	info, err := tunnelSvc.GetCloudflaredVersion(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func getCloudflaredUpdate(c *gin.Context) {
	info, err := tunnelSvc.CheckCloudflaredUpdate(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func updateCloudflared(c *gin.Context) {
	msg, err := tunnelSvc.UpdateCloudflared(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg, "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

func getAppVersion(c *gin.Context) {
	_, isDocker := os.Stat("/app/data")
	c.JSON(http.StatusOK, gin.H{"version": AppVersion, "docker": isDocker == nil})
}

func getAppUpdate(c *gin.Context) {
	req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", "https://api.github.com/repos/IYouKnow/CF-Tunnels/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	current := strings.TrimPrefix(AppVersion, "v")
	latest := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := current != "dev" && semverGreater(latest, current)
	c.JSON(http.StatusOK, gin.H{
		"currentVersion": AppVersion,
		"latestVersion":  latest,
		"hasUpdate":      hasUpdate,
		"releaseUrl":     release.HTMLURL,
	})
}

func updateApp(c *gin.Context) {
	if AppVersion == "dev" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update a dev build. Use git pull and rebuild."})
		return
	}

	req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", "https://api.github.com/repos/IYouKnow/CF-Tunnels/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check latest release: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse release data: " + err.Error()})
		return
	}

	current := strings.TrimPrefix(AppVersion, "v")
	latest := strings.TrimPrefix(release.TagName, "v")
	if latest == current {
		c.JSON(http.StatusOK, gin.H{"message": "Already up to date"})
		return
	}

	assetSuffix := runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetSuffix += ".exe"
	}
	var downloadURL string
	for _, a := range release.Assets {
		if strings.Contains(a.Name, assetSuffix) {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "No binary found for " + assetSuffix,
			"message": "Download the latest release manually: " + release.HTMLURL,
		})
		return
	}

	dlResp, err := http.Get(downloadURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Download failed: " + err.Error()})
		return
	}
	defer dlResp.Body.Close()

	exe, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot determine executable path: " + err.Error()})
		return
	}

	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot write update: " + err.Error()})
		return
	}
	if _, err := io.Copy(f, dlResp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Download write failed: " + err.Error()})
		return
	}
	f.Close()

	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot replace binary: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Update downloaded. Restarting..."})

	go func() {
		time.Sleep(500 * time.Millisecond)
		proc, _ := os.FindProcess(os.Getpid())
		if proc != nil {
			proc.Signal(syscall.SIGTERM)
		}
	}()
}

func getAppMe(c *gin.Context) {
	rawApp, _ := c.Get("app")
	rawScopes, _ := c.Get("app_scopes")
	app, _ := rawApp.(apps.App)
	scopes, _ := rawScopes.([]string)
	c.JSON(http.StatusOK, gin.H{
		"app": gin.H{
			"id":   app.ID,
			"name": app.Name,
			"slug": app.Slug,
		},
		"scopes": scopes,
	})
}

func appScopesFromContext(c *gin.Context) ([]string, bool) {
	rawScopes, exists := c.Get("app_scopes")
	if !exists {
		c.JSON(http.StatusForbidden, gin.H{"error": "Missing app scopes"})
		return nil, false
	}
	scopes, ok := rawScopes.([]string)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid app scopes"})
		return nil, false
	}
	return scopes, true
}

func hasAnyAppScope(scopes []string, required ...string) bool {
	for _, scope := range required {
		if apps.RequireScope(scopes, scope) == nil {
			return true
		}
	}
	return false
}

func ensureAppDNSRecord(c *gin.Context) {
	scopes, ok := appScopesFromContext(c)
	if !ok {
		return
	}
	if !hasAnyAppScope(scopes, "dns:read", "dns:create", "dns:update") {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing required scope: dns:read"})
		return
	}

	var req dnsservice.EnsureInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plan, err := dnsSvc.PrepareEnsure(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, dnsservice.ErrInvalidHostname):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hostname"})
		case errors.Is(err, dnsservice.ErrUnsupportedRecord):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Supported DNS record types are A, AAAA, CNAME, and TXT"})
		case errors.Is(err, dnsservice.ErrMissingContent):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		case errors.Is(err, dnsservice.ErrZoneNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching Cloudflare zone found for hostname"})
		case errors.Is(err, dnsservice.ErrCloudflareUnconfigured):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	switch plan.Action {
	case "create":
		if !hasAnyAppScope(scopes, "dns:create") {
			c.JSON(http.StatusForbidden, gin.H{"error": "missing required scope: dns:create"})
			return
		}
	case "update":
		if !hasAnyAppScope(scopes, "dns:update") {
			c.JSON(http.StatusForbidden, gin.H{"error": "missing required scope: dns:update"})
			return
		}
	default:
		if !hasAnyAppScope(scopes, "dns:read", "dns:update") {
			c.JSON(http.StatusForbidden, gin.H{"error": "missing required scope: dns:read"})
			return
		}
	}

	record, _, err := dnsSvc.EnsureRecord(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, dnsservice.ErrInvalidHostname):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hostname"})
		case errors.Is(err, dnsservice.ErrUnsupportedRecord):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Supported DNS record types are A, AAAA, CNAME, and TXT"})
		case errors.Is(err, dnsservice.ErrMissingContent):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		case errors.Is(err, dnsservice.ErrZoneNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching Cloudflare zone found for hostname"})
		case errors.Is(err, dnsservice.ErrCloudflareUnconfigured):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, record)
}

func getAppDNSRecord(c *gin.Context) {
	result, err := dnsSvc.GetRecord(c.Request.Context(), c.Param("hostname"))
	if err != nil {
		switch {
		case errors.Is(err, dnsservice.ErrInvalidHostname):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hostname"})
		case errors.Is(err, dnsservice.ErrRecordAmbiguous):
			c.JSON(http.StatusConflict, gin.H{"error": "Multiple DNS records found for hostname"})
		case errors.Is(err, dnsservice.ErrCloudflareUnconfigured):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

func patchAppDNSRecord(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	record, err := dnsSvc.UpdateRecordContent(c.Request.Context(), c.Param("hostname"), req.Content)
	if err != nil {
		switch {
		case errors.Is(err, dnsservice.ErrInvalidHostname):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hostname"})
		case errors.Is(err, dnsservice.ErrMissingContent):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		case errors.Is(err, dnsservice.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "DNS record not found"})
		case errors.Is(err, dnsservice.ErrRecordAmbiguous):
			c.JSON(http.StatusConflict, gin.H{"error": "Multiple DNS records found for hostname"})
		case errors.Is(err, dnsservice.ErrZoneNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching Cloudflare zone found for hostname"})
		case errors.Is(err, dnsservice.ErrCloudflareUnconfigured):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, record)
}

func listDNSRecords(c *gin.Context) {
	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}
	zoneID := strings.TrimSpace(c.Query("zone_id"))
	if zoneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zone_id is required"})
		return
	}
	page := 1
	perPage := 100
	if p := c.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if pp := c.Query("per_page"); pp != "" {
		perPage, _ = strconv.Atoi(pp)
	}

	records, total, err := cfClient.ListDNSRecordsByZone(c.Request.Context(), zoneID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Annotate records with matching tunnel names
	tunnelFQDN := make(map[string]string)
	rows, qErr := db.Query("SELECT subdomain, domain, name FROM tunnels WHERE zone_id = ? AND COALESCE(subdomain,'') != '' AND COALESCE(domain,'') != ''", zoneID)
	if qErr == nil {
		defer rows.Close()
		for rows.Next() {
			var sub, dom, tname string
			rows.Scan(&sub, &dom, &tname)
			if sub != "" && dom != "" {
				tunnelFQDN[sub+"."+dom] = tname
			}
		}
	}

	type recordWithTunnel struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Name       string `json:"name"`
		Content    string `json:"content"`
		Proxied    *bool  `json:"proxied,omitempty"`
		TTL        int    `json:"ttl,omitempty"`
		ZoneID     string `json:"zone_id,omitempty"`
		TunnelName string `json:"tunnel_name,omitempty"`
	}

	annotated := make([]recordWithTunnel, 0, len(records))
	for _, r := range records {
		tn := recordWithTunnel{
			ID:      r.ID,
			Type:    r.Type,
			Name:    r.Name,
			Content: r.Content,
			Proxied: r.Proxied,
			TTL:     r.TTL,
			ZoneID:  r.ZoneID,
		}
		// Match by FQDN (Cloudflare returns name with trailing dot stripped at rest)
		if name := strings.TrimSuffix(r.Name, "."); name != "" {
			if t, ok := tunnelFQDN[name]; ok {
				tn.TunnelName = t
			}
		}
		annotated = append(annotated, tn)
	}

	c.JSON(http.StatusOK, gin.H{"records": annotated, "total": total})
}

func listDomains(c *gin.Context) {
	if cfg.APIToken == "" || cfg.AccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token or account ID not configured"})
		return
	}

	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")

	result, err := cfClient.ListZones(c.Request.Context(), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"domains":  result.Domains,
		"total":    result.Total,
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
		Proxied *bool  `json:"proxied"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" {
		req.Type = "CNAME"
	}

	record, err := cfClient.CreateDNSRecord(c.Request.Context(), req.ZoneID, cloudflare.DNSRecordInput{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		Proxied: req.Proxied,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, record)
}

func updateDNSRecord(c *gin.Context) {
	zoneID := c.Param("zoneId")
	recordID := c.Param("recordId")

	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}

	var req struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Proxied *bool  `json:"proxied"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" || req.Name == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type, name, and content are required"})
		return
	}

	record, err := cfClient.UpdateDNSRecord(c.Request.Context(), zoneID, recordID, cloudflare.DNSRecordInput{
		Type:    req.Type,
		Name:    req.Name,
		Content: req.Content,
		Proxied: req.Proxied,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, record)
}

func deleteDNSRecord(c *gin.Context) {
	zoneID := c.Param("zoneId")
	recordID := c.Param("recordId")

	if cfg.APIToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare API token not configured"})
		return
	}

	if err := cfClient.DeleteDNSRecord(c.Request.Context(), zoneID, recordID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "DNS record deleted"})
}

func semverGreater(a, b string) bool {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, _ := strconv.Atoi(as[i])
		bi, _ := strconv.Atoi(bs[i])
		if ai > bi {
			return true
		}
		if ai < bi {
			return false
		}
	}
	return len(as) > len(bs)
}
