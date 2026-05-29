package tunnels

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/cf-tunnel-manager/backend/internal/cloudflare"
)

var ErrTunnelNotFound = errors.New("tunnel not found")

type BadRequestError struct {
	Message string
}

func (e *BadRequestError) Error() string {
	return e.Message
}

type CreateTunnelInput struct {
	Name      string
	AccountID string
	ZoneID    string
	Domain    string
	Subdomain string
	Address   string
}

type CreateTunnelResult struct {
	ID   int64
	Name string
}

type StartTunnelResult struct {
	PID int
}

type DeleteTunnelResult struct {
	Message  string
	Warnings []string
}

type Service struct {
	DB               *sql.DB
	CF               *cloudflare.Client
	DefaultAccountID string
	HasAPIToken      bool
	Processes        *sync.Map
	LogTunnel        func(tunnelID interface{}, level, msg string)
	NewLogWriter     func(id string, level string) io.Writer
}

type tunnelRow struct {
	ID          int
	Name        string
	UUID        string
	AccountID   string
	ZoneID      string
	Subdomain   string
	Domain      string
	Address     string
	Status      string
	PID         int
	DNSRecordID string
	TunnelToken string
}

type ingressRule struct {
	ID       int
	TunnelID int
	Hostname string
	Path     string
	Service  string
	Protocol string
}

// Service holds tunnel orchestration shared by the dashboard today and
// reusable later by an internal app-to-app API, dynamic DNS flows, and
// app-owned tunnel provisioning without duplicating handler logic.
func NewService(db *sql.DB, cf *cloudflare.Client, defaultAccountID string, hasAPIToken bool, processes *sync.Map, logTunnel func(tunnelID interface{}, level, msg string), newLogWriter func(id string, level string) io.Writer) *Service {
	return &Service{
		DB:               db,
		CF:               cf,
		DefaultAccountID: defaultAccountID,
		HasAPIToken:      hasAPIToken,
		Processes:        processes,
		LogTunnel:        logTunnel,
		NewLogWriter:     newLogWriter,
	}
}

// CreateTunnel will later be shared by dashboard routes and the internal
// Cloudflare Central API so both paths create local/remote tunnel state
// through the same orchestration rules.
func (s *Service) CreateTunnel(ctx context.Context, input CreateTunnelInput) (CreateTunnelResult, error) {
	accountID := strings.TrimSpace(input.AccountID)
	if accountID == "" {
		accountID = s.DefaultAccountID
	}

	apex, fetched, apexErr := s.resolveZoneApex(ctx, input.ZoneID, input.Domain)
	if apexErr != nil {
		log.Printf("[DNS] resolve apex at create: %v", apexErr)
	}
	if apex != "" {
		input.Domain = apex
	} else if isLikelyLegacyCorruptDomain(input.Domain) {
		input.Domain = ""
	}

	var tunnelUUID, tunnelToken string
	if input.ZoneID != "" && input.Subdomain != "" && apex != "" && s.HasAPIToken && apexErr == nil {
		if accountID == "" {
			return CreateTunnelResult{}, &BadRequestError{Message: "CF_ACCOUNT_ID is required to register a tunnel with Cloudflare for DNS"}
		}
		created, err := s.CF.CreateTunnel(ctx, accountID, input.Name)
		if err != nil {
			return CreateTunnelResult{}, &BadRequestError{Message: "Cloudflare tunnel registration failed: " + err.Error()}
		}
		tunnelUUID = created.ID
		tunnelToken = created.Token
	}

	result, err := s.DB.Exec("INSERT INTO tunnels (name, account_id, zone_id, subdomain, domain, address, uuid, tunnel_token, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'stopped')",
		input.Name, accountID, input.ZoneID, input.Subdomain, input.Domain, input.Address, tunnelUUID, tunnelToken)
	if err != nil {
		return CreateTunnelResult{}, err
	}

	id, _ := result.LastInsertId()
	s.logTunnel(id, "info", "Tunnel created")
	if fetched && apex != "" {
		s.logTunnel(id, "info", "Resolved zone apex from Cloudflare API: "+apex)
	}
	if tunnelUUID != "" {
		s.applyTunnelDNS(ctx, int(id), input.ZoneID, input.Subdomain, apex, tunnelUUID, "")
	}

	return CreateTunnelResult{ID: id, Name: input.Name}, nil
}

// StartTunnel will later be reused by dashboard routes, the internal API,
// and app-owned provisioning so tunnel bootstrapping stays in one place.
func (s *Service) StartTunnel(ctx context.Context, id int) (StartTunnelResult, error) {
	t, err := s.loadTunnelForStart(id)
	if err != nil {
		return StartTunnelResult{}, err
	}
	if t.Status == "running" && t.PID > 0 {
		return StartTunnelResult{}, &BadRequestError{Message: "Tunnel already running"}
	}

	if t.UUID == "" {
		acc := t.AccountID
		if acc == "" {
			acc = s.DefaultAccountID
		}
		if acc != "" && s.HasAPIToken {
			created, err := s.CF.CreateTunnel(ctx, acc, t.Name)
			if err != nil {
				s.logTunnel(id, "error", "Cloudflare tunnel registration failed: "+err.Error())
				return StartTunnelResult{}, &BadRequestError{Message: "Cloudflare tunnel registration failed: " + err.Error()}
			}
			t.UUID = created.ID
			t.TunnelToken = created.Token
			s.DB.Exec("UPDATE tunnels SET uuid = ?, tunnel_token = ? WHERE id = ?", t.UUID, t.TunnelToken, id)
			s.logTunnel(id, "info", "Registered tunnel with Cloudflare: "+t.UUID)
		} else {
			t.UUID = generateToken()
			s.DB.Exec("UPDATE tunnels SET uuid = ? WHERE id = ?", t.UUID, id)
			s.logTunnel(id, "info", "Generated local UUID (not a Cloudflare tunnel - DNS to .cfargotunnel.com will not work): "+t.UUID)
		}
	}

	apex, fetched, apexErr := s.resolveZoneApex(ctx, t.ZoneID, t.Domain)
	if apexErr != nil {
		log.Printf("[DNS] Could not resolve zone apex for zone_id=%s: %v", t.ZoneID, apexErr)
		s.logTunnel(id, "error", "Could not resolve zone apex: "+apexErr.Error())
	} else if fetched && apex != "" {
		t.Domain = apex
		s.DB.Exec("UPDATE tunnels SET domain = ? WHERE id = ?", apex, id)
		s.logTunnel(id, "info", "Resolved zone apex from Cloudflare API: "+apex)
	}

	log.Printf("[DNS] ZoneID=%s Subdomain=%s Apex=%s APIToken=%v", t.ZoneID, t.Subdomain, apex, s.HasAPIToken)
	s.applyTunnelDNS(ctx, id, t.ZoneID, t.Subdomain, apex, t.UUID, t.DNSRecordID)

	ingressRules, _ := s.getIngressRulesForTunnel(id)
	if t.Address != "" && len(ingressRules) == 0 {
		hostname := ""
		if t.Subdomain != "" && apex != "" {
			hostname = t.Subdomain + "." + apex
		}
		_, err := s.DB.Exec("INSERT INTO ingress_rules (tunnel_id, hostname, path, service, protocol) VALUES (?, ?, ?, ?, ?)",
			id, hostname, "", t.Address, "http")
		if err != nil {
			s.logTunnel(id, "error", "Failed to create ingress rule: "+err.Error())
		} else {
			ingressRules, _ = s.getIngressRulesForTunnel(id)
			s.logTunnel(id, "info", "Created ingress rule for: "+t.Address)
		}
	}

	if len(ingressRules) == 0 && t.Address == "" {
		return StartTunnelResult{}, &BadRequestError{Message: "No address specified and no ingress rules configured"}
	}

	acc := t.AccountID
	if acc == "" {
		acc = s.DefaultAccountID
	}
	publicHost := ""
	if strings.TrimSpace(t.Subdomain) != "" && strings.TrimSpace(apex) != "" {
		publicHost = strings.TrimSpace(t.Subdomain) + "." + strings.TrimSpace(apex)
	}
	if t.TunnelToken != "" {
		if err := s.CF.PushTunnelIngress(ctx, acc, t.UUID, toCFIngressRules(ingressRules), publicHost); err != nil {
			s.logTunnel(id, "error", "Failed to push tunnel config to Cloudflare: "+err.Error())
			return StartTunnelResult{}, fmt.Errorf("Failed to push tunnel config: %w", err)
		}
		s.logTunnel(id, "info", "Pushed ingress configuration to Cloudflare")
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

	s.logTunnel(id, "info", fmt.Sprintf("Starting tunnel with: %s", cloudflaredPath))

	var cmd *exec.Cmd
	idStr := strconv.Itoa(id)
	if t.TunnelToken != "" {
		cmd = exec.Command(cloudflaredPath, "tunnel", "run", "--token", t.TunnelToken)
	} else {
		configFile := generateConfig(t.Name, t.UUID, ingressRules)
		s.logTunnel(id, "info", "Generated config: "+configFile)
		cmd = exec.Command(cloudflaredPath, "tunnel", "--config", configFile, "run", t.UUID)
	}
	cmd.Dir = exeDir
	cmd.Stdout = s.NewLogWriter(idStr, "info")
	cmd.Stderr = s.NewLogWriter(idStr, "error")

	s.logTunnel(id, "info", "Calling cmd.Start()...")
	if err := cmd.Start(); err != nil {
		s.logTunnel(id, "error", fmt.Sprintf("Failed to start: %v", err))
		return StartTunnelResult{}, err
	}
	s.logTunnel(id, "info", "Tunnel process started")

	s.DB.Exec("UPDATE tunnels SET status = 'running', pid = ? WHERE id = ?", cmd.Process.Pid, id)
	s.Processes.Store(idStr, cmd.Process)
	s.logTunnel(id, "info", fmt.Sprintf("Tunnel started (PID: %d)", cmd.Process.Pid))

	return StartTunnelResult{PID: cmd.Process.Pid}, nil
}

// StopTunnel is kept separate from HTTP so dashboard routes and future app
// callers can share the same process-stop and state-update behavior.
func (s *Service) StopTunnel(ctx context.Context, id string) error {
	_ = ctx
	s.stopTunnelProcess(id)
	s.DB.Exec("UPDATE tunnels SET status = 'stopped', pid = 0 WHERE id = ?", id)
	s.logTunnel(id, "info", "Tunnel stopped")
	return nil
}

// DeleteTunnel will later support dashboard deletes and central-service
// resource cleanup initiated by other apps without duplicating DNS/CF cleanup.
func (s *Service) DeleteTunnel(ctx context.Context, id string) (DeleteTunnelResult, error) {
	var t struct {
		UUID        string
		DNSRecordID string
		ZoneID      string
		AccountID   string
		Subdomain   string
		Domain      string
	}
	err := s.DB.QueryRow("SELECT COALESCE(uuid, ''), COALESCE(dns_record_id, ''), COALESCE(zone_id, ''), COALESCE(account_id, ''), COALESCE(subdomain, ''), COALESCE(domain, '') FROM tunnels WHERE id = ?", id).
		Scan(&t.UUID, &t.DNSRecordID, &t.ZoneID, &t.AccountID, &t.Subdomain, &t.Domain)
	if err == sql.ErrNoRows {
		return DeleteTunnelResult{}, ErrTunnelNotFound
	}
	if err != nil {
		return DeleteTunnelResult{}, err
	}

	s.stopTunnelProcess(id)

	var warnings []string
	if t.ZoneID != "" && s.HasAPIToken {
		recordID := strings.TrimSpace(t.DNSRecordID)
		if recordID == "" && strings.TrimSpace(t.Subdomain) != "" && strings.TrimSpace(t.Domain) != "" {
			fqdn := strings.TrimSpace(t.Subdomain) + "." + strings.TrimSpace(t.Domain)
			lookedUpID, lookErr := s.CF.FindCNAMERecordID(ctx, t.ZoneID, fqdn)
			if lookErr != nil {
				warnings = append(warnings, "DNS lookup before delete failed: "+lookErr.Error())
			}
			recordID = lookedUpID
		}
		if recordID != "" {
			log.Printf("[tunnel] Deleting DNS record %s from zone %s", recordID, t.ZoneID)
			if err := s.CF.DeleteDNSRecord(ctx, t.ZoneID, recordID); err != nil {
				warnings = append(warnings, "DNS delete failed: "+err.Error())
			}
		}
	}

	if t.UUID != "" && s.HasAPIToken {
		accID := t.AccountID
		if accID == "" {
			accID = s.DefaultAccountID
		}
		if accID == "" {
			log.Printf("[tunnel] Cannot delete Cloudflare tunnel - no account_id stored and CF_ACCOUNT_ID not configured")
		} else {
			log.Printf("[tunnel] Deleting Cloudflare tunnel %s from account %s", t.UUID, accID)
			if err := s.CF.DeleteTunnel(ctx, accID, t.UUID); err != nil {
				warnings = append(warnings, "Cloudflare tunnel delete failed: "+err.Error())
			}
		}
	}

	if _, err := s.DB.Exec("DELETE FROM tunnels WHERE id = ?", id); err != nil {
		return DeleteTunnelResult{}, err
	}
	msg := "Tunnel deleted"
	if len(warnings) > 0 {
		msg += " (with warnings)"
	}
	return DeleteTunnelResult{Message: msg, Warnings: warnings}, nil
}

func (s *Service) loadTunnelForStart(id int) (tunnelRow, error) {
	var t tunnelRow
	err := s.DB.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, address, status, pid, COALESCE(dns_record_id, ''), COALESCE(tunnel_token, '') FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.Status, &t.PID, &t.DNSRecordID, &t.TunnelToken)
	if err == sql.ErrNoRows {
		return tunnelRow{}, ErrTunnelNotFound
	}
	return t, err
}

func (s *Service) getIngressRulesForTunnel(tunnelID int) ([]ingressRule, error) {
	rows, err := s.DB.Query("SELECT id, tunnel_id, hostname, path, service, protocol FROM ingress_rules WHERE tunnel_id = ?", tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ingressRule
	for rows.Next() {
		var r ingressRule
		rows.Scan(&r.ID, &r.TunnelID, &r.Hostname, &r.Path, &r.Service, &r.Protocol)
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *Service) resolveZoneApex(ctx context.Context, zoneID string, domainStored string) (apex string, fetchedFromAPI bool, err error) {
	apex = strings.TrimSpace(domainStored)
	if isLikelyLegacyCorruptDomain(apex) {
		apex = ""
	}
	if apex != "" {
		return apex, false, nil
	}
	if zoneID == "" || !s.HasAPIToken {
		return "", false, nil
	}
	name, err := s.CF.FetchZoneName(ctx, zoneID)
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

func (s *Service) applyTunnelDNS(ctx context.Context, id int, zoneID, subdomain, apex, tunnelUUID, existingDNSID string) {
	if existingDNSID != "" {
		s.logTunnel(id, "info", "DNS record already present, skipping creation")
		return
	}
	if zoneID == "" || subdomain == "" || apex == "" || !s.HasAPIToken || tunnelUUID == "" {
		s.logTunnel(id, "info", "Skipping DNS - need zone_id, subdomain, apex, tunnel UUID, and API token")
		return
	}
	fullDomain := subdomain + "." + apex
	log.Printf("[DNS] Creating CNAME: %s -> %s.cfargotunnel.com", fullDomain, tunnelUUID)
	proxied := true
	record, err := s.CF.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecordInput{
		Type:    "CNAME",
		Name:    fullDomain,
		Content: tunnelUUID + ".cfargotunnel.com",
		Proxied: &proxied,
	})
	if err != nil {
		log.Printf("[DNS] ERROR: %v", err)
		s.logTunnel(id, "error", "DNS CNAME failed: "+err.Error())
		return
	}
	if record.ID != "" {
		s.DB.Exec("UPDATE tunnels SET dns_record_id = ? WHERE id = ?", record.ID, id)
		s.logTunnel(id, "info", "DNS CNAME record created: "+fullDomain+" -> "+tunnelUUID+".cfargotunnel.com")
		log.Printf("[DNS] Created record ID: %s", record.ID)
		return
	}
	log.Printf("[DNS] Empty recordID returned")
	s.logTunnel(id, "error", "DNS CNAME returned empty recordID")
}

func (s *Service) stopTunnelProcess(id string) {
	if p, ok := s.Processes.Load(id); ok {
		proc := p.(*os.Process)
		proc.Kill()
		s.Processes.Delete(id)
	}

	var pid int
	s.DB.QueryRow("SELECT pid FROM tunnels WHERE id = ?", id).Scan(&pid)
	if pid > 0 {
		proc, _ := os.FindProcess(pid)
		if proc != nil {
			proc.Kill()
		}
		s.DB.Exec("UPDATE tunnels SET status = 'stopped', pid = 0 WHERE id = ?", id)
	}
}

func (s *Service) logTunnel(tunnelID interface{}, level, msg string) {
	if s.LogTunnel != nil {
		s.LogTunnel(tunnelID, level, msg)
	}
}

func toCFIngressRules(rules []ingressRule) []cloudflare.IngressRule {
	out := make([]cloudflare.IngressRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, cloudflare.IngressRule{
			Hostname: r.Hostname,
			Path:     r.Path,
			Service:  r.Service,
		})
	}
	return out
}

func generateConfig(name, uuid string, rules []ingressRule) string {
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
	switch scheme {
	case "http", "https":
		return (&url.URL{Scheme: u.Scheme, User: u.User, Host: u.Host}).String()
	case "tcp", "udp":
		return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	default:
		return orig
	}
}
