package main

import (
	"bytes"
	"context"
	"database/sql"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
	"github.com/spf13/viper"
)

type Config struct {
	APIToken    string `mapstructure:"CF_API_TOKEN" env:"CF_API_TOKEN"`
	AccountID   string `mapstructure:"CF_ACCOUNT_ID" env:"CF_ACCOUNT_ID"`
	AdminUser   string `mapstructure:"ADMIN_USER" env:"ADMIN_USER"`
	AdminPass   string `mapstructure:"ADMIN_PASSWORD" env:"ADMIN_PASSWORD"`
	ListenPort  int    `mapstructure:"LISTEN_PORT" env:"LISTEN_PORT"`
}

type Tunnel struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	UUID        string    `json:"uuid"`
	AccountID   string    `json:"account_id"`
	ZoneID      string    `json:"zone_id,omitempty"`
	Subdomain   string    `json:"subdomain,omitempty"`
	Domain      string    `json:"domain,omitempty"`
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

func main() {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.ReadInConfig()

	viper.Unmarshal(&cfg)

	log.Printf("Loaded config - API Token present: %v, AccountID: %s, AdminUser: %s, AdminPass: %s", cfg.APIToken != "", cfg.AccountID, cfg.AdminUser, cfg.AdminPass)

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

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(authMiddleware())

	r.GET("/api/tunnels", listTunnels)
	r.POST("/api/tunnels", createTunnel)
	r.GET("/api/tunnels/:id", getTunnel)
	r.DELETE("/api/tunnels/:id", deleteTunnel)
	r.POST("/api/tunnels/:id/start", startTunnel)
	r.POST("/api/tunnels/:id/stop", stopTunnel)
	r.GET("/api/tunnels/:id/logs", getTunnelLogs)

	r.GET("/api/ingress", listIngressRules)
	r.POST("/api/ingress", createIngressRule)
	r.PUT("/api/ingress/:id", updateIngressRule)
	r.DELETE("/api/ingress/:id", deleteIngressRule)

	r.GET("/api/status", getStatus)
	r.GET("/api/domains", listDomains)
	r.POST("/api/dns", createDNSRecord)
	r.DELETE("/api/dns/:zoneId/:recordId", deleteDNSRecord)

	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/dist/index.html")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func initDB() error {
	dbPath := "tunnels.db"
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	var err error
	db, err = sql.Open("sqlite", dbPath+"?cache=shared")
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tunnels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			uuid TEXT,
			account_id TEXT,
			zone_id TEXT,
			subdomain TEXT,
			domain TEXT,
			dns_record_id TEXT,
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
	return err
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authUser, authPass, hasAuth := c.Request.BasicAuth()
		log.Printf("Auth attempt - hasAuth: %v, user: %s, pass: %s, cfg.AdminUser: %s, cfg.AdminPass: %s", hasAuth, authUser, authPass, cfg.AdminUser, cfg.AdminPass)
		if !hasAuth {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}

		if authUser != cfg.AdminUser || authPass != cfg.AdminPass {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		c.Next()
	}
}

func listTunnels(c *gin.Context) {
	rows, err := db.Query("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, created_at, status, pid FROM tunnels")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tunnels []Tunnel
	for rows.Next() {
		var t Tunnel
		rows.Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.CreatedAt, &t.Status, &t.PID)
		tunnels = append(tunnels, t)
	}
	c.JSON(http.StatusOK, tunnels)
}

func createTunnel(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		AccountID string `json:"account_id"`
		ZoneID    string `json:"zone_id"`
		Subdomain string `json:"subdomain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AccountID == "" {
		req.AccountID = cfg.AccountID
	}

	domain := ""
	if req.ZoneID != "" && req.Subdomain != "" {
		domain = req.Subdomain + "." + req.ZoneID
	}

	result, err := db.Exec("INSERT INTO tunnels (name, account_id, zone_id, subdomain, domain, uuid, status) VALUES (?, ?, ?, ?, ?, '', 'stopped')",
		req.Name, req.AccountID, req.ZoneID, req.Subdomain, domain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := result.LastInsertId()
	logTunnel(id, "info", "Tunnel created")

	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

func getTunnel(c *gin.Context) {
	id := c.Param("id")
	var t Tunnel
	err := db.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, created_at, status, pid FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.CreatedAt, &t.Status, &t.PID)
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

	stopTunnelProcess(id)

	_, err := db.Exec("DELETE FROM tunnels WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Tunnel deleted"})
}

func startTunnel(c *gin.Context) {
	id := c.Param("id")

	var t Tunnel
	err := db.QueryRow("SELECT id, name, uuid, zone_id, subdomain, status, pid FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.ZoneID, &t.Subdomain, &t.Status, &t.PID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}

	if t.Status == "running" && t.PID > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tunnel already running"})
		return
	}

	if t.UUID == "" {
		t.UUID = generateToken()
		db.Exec("UPDATE tunnels SET uuid = ? WHERE id = ?", t.UUID, id)
	}

	if t.ZoneID != "" && t.Subdomain != "" && cfg.APIToken != "" {
		fullDomain := t.Subdomain + "." + t.ZoneID
		recordID, err := createCNAME(t.ZoneID, fullDomain, t.UUID+".cfargotunnel.com")
		if err == nil && recordID != "" {
			db.Exec("UPDATE tunnels SET dns_record_id = ? WHERE id = ?", recordID, id)
		}
		logTunnel(id, "info", "DNS CNAME record created: "+fullDomain)
	}

	ingressRules, _ := getIngressRulesForTunnel(id)
	configFile := generateConfig(t.Name, t.UUID, ingressRules)

	cmd := exec.Command("cloudflared", "tunnel", "--config", configFile, "run", t.UUID)
	cmd.Stdout = &logWriter{id: id, level: "info"}
	cmd.Stderr = &logWriter{id: id, level: "error"}

	if err := cmd.Start(); err != nil {
		logTunnel(id, "error", fmt.Sprintf("Failed to start: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	db.Exec("UPDATE tunnels SET status = 'running', pid = ? WHERE id = ?", cmd.Process.Pid, id)
	tunnelProcs.Store(id, cmd.Process)

	logTunnel(id, "info", "Tunnel started")
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

	var logs []LogEntry
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
	type Config struct {
		TunnelName string        `json:"tunnelName"`
		TunnelID   string        `json:"tunnelID"`
		 Ingress   []interface{} `json:"ingress"`
	}

	ingress := make([]interface{}, len(rules))
	for i, r := range rules {
		ingress[i] = map[string]interface{}{
			"hostname": r.Hostname,
			"service":  r.Service,
		}
		if r.Path != "" {
			ingress[i].(map[string]interface{})["path"] = r.Path
		}
	}

	cfg := Config{
		TunnelName: name,
		TunnelID:   uuid,
		Ingress:   ingress,
	}

	path := "/app/config/" + name + ".yml"
	data, _ := json.Marshal(cfg)
	os.WriteFile(path, data, 0644)
	return path
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func logTunnel(tunnelID interface{}, level, msg string) {
	db.Exec("INSERT INTO logs (tunnel_id, level, message) VALUES (?, ?, ?)", tunnelID, level, msg)
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
	log.Printf("Cloudflare API response status: %d, body: %s", resp.StatusCode, string(body))

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
	body, _ := json.Marshal(map[string]string{
		"type":    "CNAME",
		"name":    name,
		"content": content,
	})

	httpReq, _ := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/zones/"+zoneID+"/dns_records", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Result.ID, nil
}