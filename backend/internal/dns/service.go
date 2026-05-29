package dns

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cf-tunnel-manager/backend/internal/cloudflare"
)

var (
	ErrInvalidHostname        = errors.New("invalid hostname")
	ErrUnsupportedRecord      = errors.New("unsupported DNS record type")
	ErrMissingContent         = errors.New("content is required")
	ErrZoneNotFound           = errors.New("zone not found")
	ErrRecordNotFound         = errors.New("record not found")
	ErrRecordAmbiguous        = errors.New("multiple records found for hostname")
	ErrCloudflareUnconfigured = errors.New("cloudflare API token not configured")
	hostnamePattern           = regexp.MustCompile(`^(?i)[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

type Service struct {
	CF *cloudflare.Client
}

type EnsureInput struct {
	Hostname string `json:"hostname"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Proxied  *bool  `json:"proxied"`
	TTL      *int   `json:"ttl"`
	// Metadata is reserved for future ownership/audit context when app-managed
	// DNS resources are tracked beyond raw Cloudflare records.
	Metadata map[string]any `json:"metadata"`
}

type EnsurePlan struct {
	Zone     cloudflare.Zone
	Existing *cloudflare.DNSRecord
	Desired  cloudflare.DNSRecordInput
	Action   string
}

type LookupResult struct {
	Hostname string                `json:"hostname"`
	Exists   bool                  `json:"exists"`
	Record   *cloudflare.DNSRecord `json:"record"`
}

// Service centralizes DNS-only orchestration for both the dashboard-adjacent
// internal API and future app-owned integrations like dynamic DNS updates.
func NewService(cf *cloudflare.Client) *Service {
	return &Service{CF: cf}
}

func normalizeHostname(hostname string) string {
	return strings.TrimSpace(strings.ToLower(strings.TrimSuffix(hostname, ".")))
}

func validateHostname(hostname string) error {
	if !hostnamePattern.MatchString(hostname) {
		return ErrInvalidHostname
	}
	return nil
}

func normalizeType(recordType string) string {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))
	if recordType == "" {
		return "A"
	}
	return recordType
}

func validateType(recordType string) error {
	switch normalizeType(recordType) {
	case "A", "AAAA", "CNAME", "TXT":
		return nil
	default:
		return ErrUnsupportedRecord
	}
}

func normalizeTTL(ttl *int) *int {
	if ttl == nil {
		def := 120
		return &def
	}
	if *ttl <= 0 {
		def := 120
		return &def
	}
	return ttl
}

func (s *Service) ResolveZone(ctx context.Context, hostname string) (cloudflare.Zone, error) {
	if s.CF == nil || strings.TrimSpace(s.CF.APIToken) == "" {
		return cloudflare.Zone{}, ErrCloudflareUnconfigured
	}
	hostname = normalizeHostname(hostname)
	if err := validateHostname(hostname); err != nil {
		return cloudflare.Zone{}, err
	}

	parts := strings.Split(hostname, ".")
	for i := 0; i < len(parts); i++ {
		candidate := strings.Join(parts[i:], ".")
		if strings.Count(candidate, ".") < 1 {
			continue
		}
		zone, err := s.CF.FindZoneByName(ctx, candidate)
		if err != nil {
			return cloudflare.Zone{}, err
		}
		if zone != nil {
			return *zone, nil
		}
	}
	return cloudflare.Zone{}, ErrZoneNotFound
}

func (s *Service) GetRecord(ctx context.Context, hostname string) (LookupResult, error) {
	hostname = normalizeHostname(hostname)
	if err := validateHostname(hostname); err != nil {
		return LookupResult{}, err
	}
	zone, err := s.ResolveZone(ctx, hostname)
	if err != nil {
		if errors.Is(err, ErrZoneNotFound) {
			return LookupResult{Hostname: hostname, Exists: false, Record: nil}, nil
		}
		return LookupResult{}, err
	}
	records, err := s.CF.FindDNSRecords(ctx, zone.ID, hostname, "")
	if err != nil {
		return LookupResult{}, err
	}
	if len(records) == 0 {
		return LookupResult{Hostname: hostname, Exists: false, Record: nil}, nil
	}
	if len(records) > 1 {
		return LookupResult{}, ErrRecordAmbiguous
	}
	record := records[0]
	record.ZoneID = zone.ID
	return LookupResult{Hostname: hostname, Exists: true, Record: &record}, nil
}

func (s *Service) PrepareEnsure(ctx context.Context, input EnsureInput) (EnsurePlan, error) {
	hostname := normalizeHostname(input.Hostname)
	recordType := normalizeType(input.Type)
	content := strings.TrimSpace(input.Content)
	if err := validateHostname(hostname); err != nil {
		return EnsurePlan{}, err
	}
	if err := validateType(recordType); err != nil {
		return EnsurePlan{}, err
	}
	if content == "" {
		return EnsurePlan{}, ErrMissingContent
	}
	zone, err := s.ResolveZone(ctx, hostname)
	if err != nil {
		return EnsurePlan{}, err
	}

	desired := cloudflare.DNSRecordInput{
		Type:    recordType,
		Name:    hostname,
		Content: content,
		Proxied: input.Proxied,
		TTL:     normalizeTTL(input.TTL),
	}
	existing, err := s.CF.FindDNSRecord(ctx, zone.ID, hostname, recordType)
	if err != nil {
		return EnsurePlan{}, err
	}

	plan := EnsurePlan{
		Zone:     zone,
		Existing: existing,
		Desired:  desired,
		Action:   "create",
	}
	if existing == nil {
		return plan, nil
	}

	if recordMatches(*existing, desired) {
		plan.Action = "noop"
		return plan, nil
	}
	plan.Action = "update"
	return plan, nil
}

func recordMatches(existing cloudflare.DNSRecord, desired cloudflare.DNSRecordInput) bool {
	if !strings.EqualFold(existing.Type, desired.Type) {
		return false
	}
	if normalizeHostname(existing.Name) != normalizeHostname(desired.Name) {
		return false
	}
	if strings.TrimSpace(existing.Content) != strings.TrimSpace(desired.Content) {
		return false
	}
	if desired.Proxied != nil {
		if existing.Proxied == nil || *existing.Proxied != *desired.Proxied {
			return false
		}
	}
	if desired.TTL != nil && existing.TTL != 0 && existing.TTL != *desired.TTL {
		return false
	}
	return true
}

func (s *Service) EnsureRecord(ctx context.Context, input EnsureInput) (cloudflare.DNSRecord, string, error) {
	plan, err := s.PrepareEnsure(ctx, input)
	if err != nil {
		return cloudflare.DNSRecord{}, "", err
	}

	// TODO: Restrict DNS writes to app-owned hostnames/resources once ownership tracking exists.
	switch plan.Action {
	case "create":
		record, err := s.CF.CreateDNSRecord(ctx, plan.Zone.ID, plan.Desired)
		return record, plan.Action, err
	case "update":
		record, err := s.CF.UpdateDNSRecord(ctx, plan.Zone.ID, plan.Existing.ID, plan.Desired)
		return record, plan.Action, err
	case "noop":
		record := *plan.Existing
		record.ZoneID = plan.Zone.ID
		return record, plan.Action, nil
	default:
		return cloudflare.DNSRecord{}, "", fmt.Errorf("unknown ensure action")
	}
}

func (s *Service) UpdateRecordContent(ctx context.Context, hostname string, content string) (cloudflare.DNSRecord, error) {
	hostname = normalizeHostname(hostname)
	content = strings.TrimSpace(content)
	if err := validateHostname(hostname); err != nil {
		return cloudflare.DNSRecord{}, err
	}
	if content == "" {
		return cloudflare.DNSRecord{}, ErrMissingContent
	}
	zone, err := s.ResolveZone(ctx, hostname)
	if err != nil {
		return cloudflare.DNSRecord{}, err
	}
	records, err := s.CF.FindDNSRecords(ctx, zone.ID, hostname, "")
	if err != nil {
		return cloudflare.DNSRecord{}, err
	}
	if len(records) == 0 {
		return cloudflare.DNSRecord{}, ErrRecordNotFound
	}
	if len(records) > 1 {
		return cloudflare.DNSRecord{}, ErrRecordAmbiguous
	}
	record := records[0]
	updated, err := s.CF.UpdateDNSRecord(ctx, zone.ID, record.ID, cloudflare.DNSRecordInput{
		Type:    record.Type,
		Name:    record.Name,
		Content: content,
		Proxied: record.Proxied,
		TTL:     ttlPointer(record.TTL),
	})
	if err != nil {
		return cloudflare.DNSRecord{}, err
	}
	return updated, nil
}

func ttlPointer(ttl int) *int {
	if ttl <= 0 {
		return nil
	}
	value := ttl
	return &value
}
