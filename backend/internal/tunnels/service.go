package tunnels

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	// AutoStart controls whether the tunnel is respawned on container boot.
	// Nil means "default true" to preserve existing behavior.
	AutoStart *bool
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
	AutoStart   bool
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

	result, err := s.DB.Exec("INSERT INTO tunnels (name, account_id, zone_id, subdomain, domain, address, uuid, tunnel_token, status, auto_start) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'stopped', ?)",
		input.Name, accountID, input.ZoneID, input.Subdomain, input.Domain, input.Address, tunnelUUID, tunnelToken, boolToInt(autoStartOrDefault(input.AutoStart)))
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

func (s *Service) UpdateTunnelName(ctx context.Context, id int, newName string) error {
	var uuid string
	var accountID string
	err := s.DB.QueryRow("SELECT COALESCE(uuid,''), COALESCE(account_id,'') FROM tunnels WHERE id = ?", id).Scan(&uuid, &accountID)
	if err == sql.ErrNoRows {
		return ErrTunnelNotFound
	}
	if err != nil {
		return err
	}

	// If the tunnel exists on Cloudflare, rename it there first.
	if uuid != "" && s.HasAPIToken {
		acc := accountID
		if acc == "" {
			acc = s.DefaultAccountID
		}
		if acc != "" {
			if err := s.CF.UpdateTunnelName(ctx, acc, uuid, newName); err != nil {
				return fmt.Errorf("cloudflare rename failed: %w", err)
			}
		}
	}

	_, err = s.DB.Exec("UPDATE tunnels SET name = ? WHERE id = ?", newName, id)
	return err
}

// UpdateTunnelSettings renames and/or toggles auto_start in one call.
func (s *Service) UpdateTunnelSettings(ctx context.Context, id int, newName string, autoStart *bool) error {
	if strings.TrimSpace(newName) != "" {
		if err := s.UpdateTunnelName(ctx, id, strings.TrimSpace(newName)); err != nil {
			return err
		}
	}
	if autoStart != nil {
		if err := s.SetAutoStart(id, *autoStart); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SyncTunnels(ctx context.Context) (imported int, updated int, err error) {
	if !s.HasAPIToken {
		return 0, 0, fmt.Errorf("Cloudflare API token not configured")
	}
	cfTunnels, err := s.CF.ListTunnels(ctx, s.DefaultAccountID)
	if err != nil {
		return 0, 0, err
	}

	zoneMap := make([]cloudflare.Zone, 0)
	if zr, zErr := s.CF.ListZones(ctx, "1", "100"); zErr == nil {
		zoneMap = zr.Domains
		for pg := 2; len(zoneMap) < zr.Total; pg++ {
			more, mErr := s.CF.ListZones(ctx, fmt.Sprintf("%d", pg), "100")
			if mErr != nil {
				break
			}
			zoneMap = append(zoneMap, more.Domains...)
		}
	} else {
		log.Printf("[sync] ListZones failed (zones needed for hostname resolution): %v", zErr)
	}

	imported = 0
	updated = 0
	for _, t := range cfTunnels {
		cfStatus := "stopped"
		if t.Status == "healthy" || t.Status == "degraded" {
			cfStatus = "running"
		}

		var existingID int
		err := s.DB.QueryRow("SELECT id FROM tunnels WHERE uuid = ?", t.ID).Scan(&existingID)
		if err == sql.ErrNoRows {
			var zoneID, subdomain, domain, address string
			cfg, cfgErr := s.CF.GetTunnelConfig(ctx, s.DefaultAccountID, t.ID)
			if cfgErr != nil {
				log.Printf("[sync] GetTunnelConfig failed for %s (%s): %v", t.Name, t.ID, cfgErr)
			}
			if cfgErr == nil && cfg != nil {
				for _, rule := range cfg.Ingress {
					if rule.Hostname != "" && rule.Service != "" && !strings.HasPrefix(rule.Service, "http_status:") {
						hostname := rule.Hostname
						address = rule.Service
						if zid, dm, sub := matchZoneForHostname(hostname, zoneMap); zid != "" {
							zoneID = zid
							domain = dm
							subdomain = sub
						} else {
							domain = hostname
						}
						break
					}
				}
			}

			_, insertErr := s.DB.Exec(
				"INSERT INTO tunnels (name, uuid, account_id, zone_id, subdomain, domain, address, status, auto_start, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)",
				t.Name, t.ID, t.AccountID, zoneID, subdomain, domain, address, cfStatus, t.CreatedAt,
			)
			if insertErr != nil {
				log.Printf("[sync] Failed to import tunnel %q: %v", t.Name, insertErr)
				continue
			}
			imported++
			s.logTunnel(t.Name, "info", "Imported from Cloudflare")
		} else {
			// Update status for existing tunnels
			s.DB.Exec("UPDATE tunnels SET status = ? WHERE id = ?", cfStatus, existingID)

			// Fill in missing domain/address from ingress config
			var currentAddress, currentDomain string
			s.DB.QueryRow("SELECT COALESCE(address,''), COALESCE(domain,'') FROM tunnels WHERE id = ?", existingID).Scan(&currentAddress, &currentDomain)
			if currentAddress == "" || currentDomain == "" {
				cfg, cfgErr := s.CF.GetTunnelConfig(ctx, s.DefaultAccountID, t.ID)
				if cfgErr != nil {
					log.Printf("[sync] GetTunnelConfig for existing tunnel %s (%s): %v", t.Name, t.ID, cfgErr)
				}
				if cfgErr == nil && cfg != nil {
					for _, rule := range cfg.Ingress {
						if rule.Hostname != "" && rule.Service != "" && !strings.HasPrefix(rule.Service, "http_status:") {
							if currentAddress == "" {
								currentAddress = rule.Service
							}
							if currentDomain == "" {
								if zid, dm, sub := matchZoneForHostname(rule.Hostname, zoneMap); zid != "" {
									s.DB.Exec("UPDATE tunnels SET zone_id = ?, subdomain = ?, domain = ?, address = ? WHERE id = ?", zid, sub, dm, currentAddress, existingID)
								} else {
									s.DB.Exec("UPDATE tunnels SET domain = ?, address = ? WHERE id = ?", rule.Hostname, currentAddress, existingID)
								}
							}
							break
						}
					}
				}
			}
			updated++
		}
	}
	return imported, updated, nil
}

func matchZoneForHostname(hostname string, zones []cloudflare.Zone) (zoneID, domain, subdomain string) {
	hostname = strings.TrimSuffix(hostname, ".")
	parts := strings.Split(hostname, ".")
	for i := 1; i < len(parts)-1; i++ {
		candidate := strings.Join(parts[i:], ".")
		for _, z := range zones {
			if strings.EqualFold(z.Name, candidate) {
				return z.ID, z.Name, strings.Join(parts[:i], ".")
			}
		}
	}
	return "", "", ""
}

// StartTunnel will later be reused by dashboard routes, the internal API,
// and app-owned provisioning so tunnel bootstrapping stays in one place.
func (s *Service) StartTunnel(ctx context.Context, id int) (StartTunnelResult, error) {
	t, err := s.loadTunnelForStart(id)
	if err != nil {
		return StartTunnelResult{}, err
	}
	if t.Status == "running" && s.isTunnelAlive(t) {
		return StartTunnelResult{}, &BadRequestError{Message: "Tunnel already running"}
	}
	// Stale row (status=running but no live process, e.g. after `docker restart`):
	// fall through and respawn instead of returning "already running".
	//
	// NOTE: multiple tunnels may legitimately share one destination address
	// (each has its own UUID, CNAME and connector), so no address-uniqueness
	// check is enforced here.

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

	var exists bool
	err := s.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM tunnels WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check tunnel: %w", err)
	}
	if !exists {
		return ErrTunnelNotFound
	}

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
	// Belt-and-suspenders: FK cascade needs PRAGMA foreign_keys=ON per
	// connection, so explicitly remove rules to avoid orphans (RCA A6).
	_, _ = s.DB.Exec("DELETE FROM ingress_rules WHERE tunnel_id NOT IN (SELECT id FROM tunnels)")
	msg := "Tunnel deleted"
	if len(warnings) > 0 {
		msg += " (with warnings)"
	}
	return DeleteTunnelResult{Message: msg, Warnings: warnings}, nil
}

func (s *Service) loadTunnelForStart(id int) (tunnelRow, error) {
	var t tunnelRow
	var autoStart sql.NullInt64
	err := s.DB.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, address, status, pid, COALESCE(dns_record_id, ''), COALESCE(tunnel_token, ''), auto_start FROM tunnels WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.UUID, &t.AccountID, &t.ZoneID, &t.Subdomain, &t.Domain, &t.Address, &t.Status, &t.PID, &t.DNSRecordID, &t.TunnelToken, &autoStart)
	if err == sql.ErrNoRows {
		return tunnelRow{}, ErrTunnelNotFound
	}
	if err != nil {
		// Older DBs without the auto_start column: retry without it, default true.
		var t2 tunnelRow
		err2 := s.DB.QueryRow("SELECT id, name, uuid, account_id, zone_id, subdomain, domain, address, status, pid, COALESCE(dns_record_id, ''), COALESCE(tunnel_token, '') FROM tunnels WHERE id = ?", id).
			Scan(&t2.ID, &t2.Name, &t2.UUID, &t2.AccountID, &t2.ZoneID, &t2.Subdomain, &t2.Domain, &t2.Address, &t2.Status, &t2.PID, &t2.DNSRecordID, &t2.TunnelToken)
		if err2 != nil {
			return tunnelRow{}, err2
		}
		t2.AutoStart = true
		return t2, nil
	}
	t.AutoStart = !autoStart.Valid || autoStart.Int64 != 0
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
	if zoneID == "" || subdomain == "" || apex == "" || !s.HasAPIToken || tunnelUUID == "" {
		s.logTunnel(id, "info", "Skipping DNS - need zone_id, subdomain, apex, tunnel UUID, and API token")
		return
	}
	fullDomain := subdomain + "." + apex
	expectedContent := tunnelUUID + ".cfargotunnel.com"
	if strings.TrimSpace(existingDNSID) != "" {
		// Validate the stored record instead of blindly trusting it (RCA:
		// manual delete/recreate left dns_record_id mismatched).
		records, err := s.CF.FindDNSRecords(ctx, zoneID, fullDomain, "CNAME")
		if err != nil {
			s.logTunnel(id, "error", "DNS validation lookup failed, keeping stored record: "+err.Error())
			log.Printf("[DNS] validation lookup failed for %s: %v", fullDomain, err)
			return
		}
		for _, r := range records {
			if r.ID == strings.TrimSpace(existingDNSID) && strings.EqualFold(strings.TrimSuffix(r.Content, "."), expectedContent) {
				s.logTunnel(id, "info", "DNS record already present, skipping creation")
				return
			}
		}
		s.logTunnel(id, "info", "Stored DNS record missing/mismatched vs Cloudflare, recreating CNAME")
		log.Printf("[DNS] stored record %s missing/mismatched for %s, recreating", existingDNSID, fullDomain)
	}
	log.Printf("[DNS] Creating CNAME: %s -> %s.cfargotunnel.com", fullDomain, tunnelUUID)
	proxied := true
	record, err := s.CF.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecordInput{
		Type:    "CNAME",
		Name:    fullDomain,
		Content: tunnelUUID + ".cfargotunnel.com",
		Proxied: &proxied,
	})
	if err != nil {
		// The record may already exist in Cloudflare while our DB has no
		// (or a stale) dns_record_id — e.g. local dev DB against the real
		// CF account. Adopt the existing record instead of erroring every boot.
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			if existing, findErr := s.CF.FindDNSRecord(ctx, zoneID, fullDomain, "CNAME"); findErr == nil && existing != nil {
				s.DB.Exec("UPDATE tunnels SET dns_record_id = ? WHERE id = ?", existing.ID, id)
				if strings.EqualFold(strings.TrimSuffix(existing.Content, "."), expectedContent) {
					s.logTunnel(id, "info", "Adopted existing DNS CNAME record: "+fullDomain)
					log.Printf("[DNS] Adopted existing record %s for %s", existing.ID, fullDomain)
				} else {
					s.logTunnel(id, "error", fmt.Sprintf("DNS CNAME %s points to %q, expected %q — fix manually in Cloudflare",
						fullDomain, existing.Content, expectedContent))
					log.Printf("[DNS] WARNING: %s points to %q, expected %q", fullDomain, existing.Content, expectedContent)
				}
				return
			}
		}
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
		proc.Signal(os.Interrupt)
		s.Processes.Delete(id)
	}

	var pid int
	s.DB.QueryRow("SELECT pid FROM tunnels WHERE id = ?", id).Scan(&pid)
	if pid > 0 && isPIDAlive(pid) {
		if proc, err := os.FindProcess(pid); err == nil && proc != nil {
			proc.Signal(os.Interrupt)
		}
	}
	s.DB.Exec("UPDATE tunnels SET status = 'stopped', pid = 0 WHERE id = ?", id)
}

func (s *Service) StopAll() {
	// NOTE: intentionally in-memory only — DB keeps status='running' as the
	// desired state so a plain `docker restart` (SIGTERM + start) can respawn
	// via ReconcileOnBoot. Explicit user stops go through StopTunnel which
	// persists status='stopped'.
	s.Processes.Range(func(key, value any) bool {
		proc := value.(*os.Process)
		proc.Signal(os.Interrupt)
		s.Processes.Delete(key)
		return true
	})
}

// isTunnelAlive reports whether a tunnel row has a live cloudflared child.
// os.FindProcess alone always succeeds on Linux, so we also check the
// in-memory process table, Signal(0), and /proc/<pid>.
func (s *Service) isTunnelAlive(t tunnelRow) bool {
	if t.PID <= 0 {
		return false
	}
	if _, ok := s.Processes.Load(strconv.Itoa(t.ID)); ok {
		return isPIDAlive(t.PID)
	}
	// After a restart the map is empty; fall back to OS-level liveness.
	// A foreign process could theoretically reuse the pid, so callers
	// respawn when in doubt — false negatives self-heal, false positives
	// would wedge the tunnel as "already running".
	return isPIDAlive(t.PID)
}

func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		return false
	}
	if runtime.GOOS != "windows" {
		// Signal 0 performs error checking without delivering a signal.
		// ESRCH => no such process; EPERM => process exists (no permission).
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return false
		}
		// Guard against zombies: /proc/<pid>/stat state == Z means dead.
		if stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); err == nil {
			if i := bytes.LastIndexByte(stat, ')'); i >= 0 && i+2 < len(stat) && stat[i+2] == 'Z' {
				return false
			}
		} else if !os.IsPermission(err) {
			// No /proc entry and no permission issue => process gone
			// (on systems with /proc; on macOS ReadFile fails differently
			// but Signal(0) above already validated).
			if _, err2 := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err2 != nil && runtime.GOOS == "linux" {
				return false
			}
		}
	}
	return true
}

// LiveCounts returns desired (DB status=running) vs actually-alive counts.
func (s *Service) LiveCounts() (desired, alive, total int) {
	rows, err := s.DB.Query("SELECT id, status, pid FROM tunnels")
	if err != nil {
		return 0, 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var id, pid int
		var status string
		if err := rows.Scan(&id, &status, &pid); err != nil {
			continue
		}
		total++
		if status != "running" {
			continue
		}
		desired++
		if _, ok := s.Processes.Load(strconv.Itoa(id)); ok && isPIDAlive(pid) {
			alive++
		} else if pid > 0 && isPIDAlive(pid) {
			// Edge: pid alive but not in our map (e.g. adopted). Count it.
			alive++
		}
	}
	return desired, alive, total
}

type ReconcileResult struct {
	Total   int      `json:"total"`
	Started int      `json:"started"`
	Skipped int      `json:"skipped"`
	Failed  []string `json:"failed"`
	Pruned  int64    `json:"prunedOrphans"`
}

// PruneOrphanIngressRules deletes ingress_rules referencing deleted tunnels.
func (s *Service) PruneOrphanIngressRules() (int64, error) {
	res, err := s.DB.Exec("DELETE FROM ingress_rules WHERE tunnel_id NOT IN (SELECT id FROM tunnels)")
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[BOOT] pruned %d orphan ingress_rules", n)
	}
	return n, nil
}

// ReconcileOnBoot respawns cloudflared children for tunnels whose desired
// state is running+auto_start but which have no live process (e.g. after a
// plain `docker restart`). It also refreshes stale pid/status rows and prunes
// orphan ingress rules.
func (s *Service) ReconcileOnBoot(ctx context.Context) ReconcileResult {
	var res ReconcileResult
	pruned, _ := s.PruneOrphanIngressRules()
	res.Pruned = pruned

	rows, err := s.DB.Query("SELECT id, name, status, pid, COALESCE(auto_start, 1) FROM tunnels")
	if err != nil {
		log.Printf("[BOOT] reconcile: query failed: %v", err)
		return res
	}
	defer rows.Close()

	type candidate struct {
		id        int
		name      string
		status    string
		pid       int
		autoStart bool
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var auto int64
		if err := rows.Scan(&c.id, &c.name, &c.status, &c.pid, &auto); err != nil {
			continue
		}
		c.autoStart = auto != 0
		candidates = append(candidates, c)
	}
	res.Total = len(candidates)

	for _, c := range candidates {
		if c.status != "running" || !c.autoStart {
			// Clear dead pids on rows that should stay stopped.
			if c.pid != 0 && !isPIDAlive(c.pid) {
				s.DB.Exec("UPDATE tunnels SET pid = 0 WHERE id = ?", c.id)
			}
			res.Skipped++
			continue
		}
		idStr := strconv.Itoa(c.id)
		if _, ok := s.Processes.Load(idStr); ok && isPIDAlive(c.pid) {
			res.Skipped++
			continue
		}
		if c.pid > 0 && isPIDAlive(c.pid) {
			// Adopted live pid (shouldn't normally happen after restart).
			res.Skipped++
			continue
		}
		// Stale pid row — reset before respawn so retries/observability are clean.
		s.DB.Exec("UPDATE tunnels SET pid = 0 WHERE id = ?", c.id)
		s.logTunnel(c.id, "info", "Boot reconcile: respawning missing cloudflared child (post-restart)")
		startCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		_, err := s.StartTunnel(startCtx, c.id)
		cancel()
		if err != nil {
			msg := fmt.Sprintf("%s(id=%d): %v", c.name, c.id, err)
			res.Failed = append(res.Failed, msg)
			s.logTunnel(c.id, "error", "Boot reconcile failed: "+err.Error())
			log.Printf("[BOOT] reconcile: start %s failed: %v", msg, err)
			continue
		}
		res.Started++
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("[BOOT] reconcile done: started %d/%d (skipped=%d failed=%d prunedOrphans=%d)",
		res.Started, res.Total, res.Skipped, len(res.Failed), res.Pruned)
	s.logTunnel(nil, "info", fmt.Sprintf("Boot reconcile: started %d/%d tunnels (skipped=%d failed=%d prunedOrphans=%d)",
		res.Started, res.Total, res.Skipped, len(res.Failed), res.Pruned))
	return res
}

// SetAutoStart toggles per-tunnel respawn-on-boot without changing run state.
func (s *Service) SetAutoStart(id int, enabled bool) error {
	var exists bool
	if err := s.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM tunnels WHERE id = ?)", id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrTunnelNotFound
	}
	_, err := s.DB.Exec("UPDATE tunnels SET auto_start = ? WHERE id = ?", boolToInt(enabled), id)
	if err != nil {
		return err
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	s.logTunnel(id, "info", "Auto-start "+state)
	return nil
}

func autoStartOrDefault(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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

func resolveCloudflaredPath() string {
	exeDir, _ := filepath.Abs(".")
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName = "cloudflared.exe"
	}
	path := filepath.Join(exeDir, binName)
	if _, err := os.Stat(path); err != nil {
		path = binName
	}
	return path
}

type VersionInfo struct {
	Version string `json:"version"`
	Raw     string `json:"raw"`
}

type UpdateInfo struct {
	LatestVersion  string `json:"latestVersion"`
	CurrentVersion string `json:"currentVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl"`
}

func (s *Service) GetCloudflaredVersion(ctx context.Context) (*VersionInfo, error) {
	path := resolveCloudflaredPath()
	cmd := exec.CommandContext(ctx, path, "version")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run cloudflared version: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	line := strings.SplitN(raw, "\n", 2)[0]
	ver := strings.TrimPrefix(line, "cloudflared version ")
	return &VersionInfo{Version: ver, Raw: raw}, nil
}

func (s *Service) CheckCloudflaredUpdate(ctx context.Context) (*UpdateInfo, error) {
	current, err := s.GetCloudflaredVersion(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/cloudflare/cloudflared/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check latest release: %w", err)
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := latest != current.Version && current.Version != ""

	return &UpdateInfo{
		LatestVersion:  latest,
		CurrentVersion: current.Version,
		HasUpdate:      hasUpdate,
		ReleaseURL:     release.HTMLURL,
	}, nil
}

func (s *Service) UpdateCloudflared(ctx context.Context) (string, error) {
	path := resolveCloudflaredPath()
	cmd := exec.CommandContext(ctx, path, "update")
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(msg, "has been updated") || strings.Contains(msg, "up to date") {
			return msg, nil
		}
		return msg, fmt.Errorf("update failed: %w", err)
	}
	return msg, nil
}
