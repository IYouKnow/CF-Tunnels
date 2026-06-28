package cloudflare

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.cloudflare.com/client/v4"

type Client struct {
	APIToken   string
	AccountID  string
	HTTPClient *http.Client
}

type IngressRule struct {
	Hostname string
	Path     string
	Service  string
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
	Proxied *bool  `json:"proxied,omitempty"`
	TTL     int    `json:"ttl,omitempty"`
	ZoneID  string `json:"zone_id,omitempty"`
}

type DNSRecordInput struct {
	Type    string
	Name    string
	Content string
	Proxied *bool
	TTL     *int
}

type ListZonesResult struct {
	Domains []Zone
	Total   int
	Page    int
	PerPage int
}

type CreateTunnelResult struct {
	ID    string
	Token string
}

type TunnelItem struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	AccountID string     `json:"account_tag"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// Client is the first Cloudflare service seam for CF Tunnels.
// Later this layer can enforce app-owned resources, broker central API calls
// from other self-hosted apps, and support dynamic DNS/tunnel provisioning.
func NewClient(apiToken string, accountID string) *Client {
	return &Client{
		APIToken:  strings.TrimSpace(strings.Trim(apiToken, `"'`)),
		AccountID: strings.TrimSpace(strings.Trim(accountID, `"'`)),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) doJSON(ctx context.Context, method, rawURL string, body any, out any) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp, respBody, err
		}
	}
	return resp, respBody, nil
}

func firstErrorMessage(raw []byte, fallback string, messages ...string) string {
	for _, msg := range messages {
		if strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	if text := strings.TrimSpace(string(raw)); text != "" {
		return text
	}
	return fallback
}

func (c *Client) ValidateAccountID(ctx context.Context, accountID string) error {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(c.APIToken) == "" {
		return nil
	}

	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	resp, _, err := c.doJSON(ctx, http.MethodGet, baseURL+"/accounts?per_page=50", nil, &out)
	if err != nil {
		return fmt.Errorf("could not list Cloudflare accounts: %w", err)
	}
	if resp.StatusCode >= 400 || !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return fmt.Errorf("token cannot list accounts (HTTP %d). Add permission: Account -> Account Settings -> Read, or widen Account resources on the token. %s",
			resp.StatusCode, strings.TrimSpace(msg))
	}
	for _, a := range out.Result {
		if a.ID == accountID {
			return nil
		}
	}
	ids := make([]string, 0, 5)
	for _, a := range out.Result {
		ids = append(ids, a.ID)
		if len(ids) >= 5 {
			break
		}
	}
	return fmt.Errorf("CF_ACCOUNT_ID does not match any account this token can access (first IDs: %v). Fix CF_ACCOUNT_ID in .env or edit the token so Account resources include that account (or All accounts)", ids)
}

func (c *Client) CreateTunnel(ctx context.Context, accountID string, tunnelName string) (CreateTunnelResult, error) {
	if accountID == "" {
		return CreateTunnelResult{}, fmt.Errorf("account ID is empty")
	}
	if c.APIToken == "" {
		return CreateTunnelResult{}, fmt.Errorf("API token is empty")
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

		resp, raw, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/accounts/%s/cfd_tunnel", baseURL, accountID), map[string]string{
			"name":       cfName,
			"config_src": "cloudflare",
		}, &out)
		if err != nil {
			return CreateTunnelResult{}, err
		}
		if out.Success && len(out.Errors) == 0 && out.Result.ID != "" && out.Result.Token != "" {
			return CreateTunnelResult{ID: out.Result.ID, Token: out.Result.Token}, nil
		}

		msg := "unknown error"
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		if resp.StatusCode == http.StatusConflict {
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			strings.EqualFold(msg, "Authentication error") ||
			strings.Contains(strings.ToLower(msg), "auth") {
			accHint := ""
			if err := c.ValidateAccountID(ctx, accountID); err != nil {
				accHint = " " + err.Error()
			}
			return CreateTunnelResult{}, fmt.Errorf("%s (HTTP %d).%s Also ensure the token has Cloudflare One Connector: cloudflared - Write (tunnel registration is not allowed with Zone DNS-only tokens).",
				msg, resp.StatusCode, accHint)
		}
		return CreateTunnelResult{}, fmt.Errorf("%s (HTTP %d)", firstErrorMessage(raw, msg), resp.StatusCode)
	}

	return CreateTunnelResult{}, fmt.Errorf("could not register tunnel after retries: Cloudflare keeps returning 409 (name conflict). Delete the old tunnel in Zero Trust -> Networks -> Tunnels, or pick another tunnel name in this app")
}

func (c *Client) ListTunnels(ctx context.Context, accountID string) ([]TunnelItem, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account ID is empty")
	}
	if c.APIToken == "" {
		return nil, fmt.Errorf("API token is empty")
	}

	var all []TunnelItem
	page := 1
	for {
		var out struct {
			Success bool          `json:"success"`
			Result  []TunnelItem  `json:"result"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
			ResultInfo struct {
				TotalCount int `json:"total_count"`
			} `json:"result_info"`
		}

		url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel?page=%d&per_page=100", baseURL, accountID, page)
		_, raw, err := c.doJSON(ctx, http.MethodGet, url, nil, &out)
		if err != nil {
			return nil, err
		}
		if !out.Success {
			msg := ""
			if len(out.Errors) > 0 {
				msg = out.Errors[0].Message
			}
			return nil, fmt.Errorf(firstErrorMessage(raw, "Cloudflare tunnel list failed", msg))
		}

		for _, t := range out.Result {
			if t.DeletedAt == nil {
				all = append(all, t)
			}
		}

		if len(out.Result) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) DeleteTunnel(ctx context.Context, accountID string, tunnelID string) error {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		var out struct {
			Success bool `json:"success"`
			Errors  []struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		_, raw, err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s", baseURL, accountID, tunnelID), nil, &out)
		if err != nil {
			lastErr = err
		} else if out.Success {
			return nil
		} else {
			msg := firstErrorMessage(raw, "unknown error")
			code := 0
			if len(out.Errors) > 0 {
				code = out.Errors[0].Code
				msg = firstErrorMessage(raw, msg, out.Errors[0].Message)
			}
			lastErr = fmt.Errorf("%s", strings.TrimSpace(msg))
			if code == 1022 && attempt < 5 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return lastErr
		}

		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}

func (c *Client) UpdateTunnelName(ctx context.Context, accountID string, tunnelID string, newName string) error {
	if accountID == "" || tunnelID == "" {
		return fmt.Errorf("account ID and tunnel ID are required")
	}
	if c.APIToken == "" {
		return fmt.Errorf("API token is empty")
	}
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s", baseURL, accountID, tunnelID), map[string]string{
		"name": sanitizeTunnelNameForCF(newName),
	}, &out)
	if err != nil {
		return err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return fmt.Errorf(firstErrorMessage(raw, "rename tunnel failed", msg))
	}
	return nil
}

func (c *Client) PushTunnelIngress(ctx context.Context, accountID string, tunnelID string, rules []IngressRule, publicHost string) error {
	if accountID == "" || tunnelID == "" {
		return fmt.Errorf("account ID and tunnel ID are required")
	}
	publicHost = normalizeIngressHostname(publicHost)

	type cfIngress struct {
		Hostname string `json:"hostname,omitempty"`
		Path     string `json:"path,omitempty"`
		Service  string `json:"service"`
	}

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

	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Messages []struct {
			Message string `json:"message"`
		} `json:"messages"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", baseURL, accountID, tunnelID), map[string]any{
		"config": map[string]any{
			"ingress": ingress,
		},
	}, &out)
	if err != nil {
		return err
	}
	if !out.Success || len(out.Errors) > 0 {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		parts := []string{firstErrorMessage(raw, "unknown error", msg)}
		for _, m := range out.Messages {
			if strings.TrimSpace(m.Message) != "" {
				parts = append(parts, strings.TrimSpace(m.Message))
			}
		}
		return fmt.Errorf(strings.Join(parts, " | "))
	}
	return nil
}

type TunnelConfigIngress struct {
	Hostname string `json:"hostname,omitempty"`
	Path     string `json:"path,omitempty"`
	Service  string `json:"service"`
}

type TunnelConfigResult struct {
	Ingress []TunnelConfigIngress `json:"ingress"`
}

func (c *Client) GetTunnelConfig(ctx context.Context, accountID string, tunnelID string) (*TunnelConfigResult, error) {
	if accountID == "" || tunnelID == "" {
		return nil, fmt.Errorf("account ID and tunnel ID are required")
	}
	var out struct {
		Success bool `json:"success"`
		Result  struct {
			Config TunnelConfigResult `json:"config"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", baseURL, accountID, tunnelID), nil, &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return nil, fmt.Errorf(firstErrorMessage(raw, "failed to get tunnel config", msg))
	}
	return &out.Result.Config, nil
}

func (c *Client) ListZones(ctx context.Context, page string, perPage string) (ListZonesResult, error) {
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result     []Zone `json:"result"`
		ResultInfo struct {
			TotalCount int `json:"total_count"`
		} `json:"result_info"`
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/zones?page=%s&per_page=%s", baseURL, page, perPage), nil, &out)
	if err != nil {
		return ListZonesResult{}, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return ListZonesResult{}, fmt.Errorf(firstErrorMessage(raw, "Cloudflare zone list failed", msg))
	}
	return ListZonesResult{
		Domains: out.Result,
		Total:   out.ResultInfo.TotalCount,
		Page:    out.Page,
		PerPage: out.PerPage,
	}, nil
}

func (c *Client) FetchZoneName(ctx context.Context, zoneID string) (string, error) {
	var out struct {
		Success bool `json:"success"`
		Result  struct {
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/zones/%s", baseURL, zoneID), nil, &out)
	if err != nil {
		return "", err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return "", fmt.Errorf(firstErrorMessage(raw, "zone lookup failed", msg))
	}
	if out.Result.Name == "" {
		return "", fmt.Errorf("empty zone name")
	}
	return out.Result.Name, nil
}

func (c *Client) FindZoneByName(ctx context.Context, zoneName string) (*Zone, error) {
	zoneName = strings.TrimSpace(strings.ToLower(strings.TrimSuffix(zoneName, ".")))
	if zoneName == "" {
		return nil, nil
	}

	reqURL := fmt.Sprintf("%s/zones?name=%s", baseURL, url.QueryEscape(zoneName))
	var out struct {
		Success bool   `json:"success"`
		Result  []Zone `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, reqURL, nil, &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return nil, fmt.Errorf(firstErrorMessage(raw, "zone lookup failed", msg))
	}
	for _, zone := range out.Result {
		if strings.EqualFold(strings.TrimSpace(zone.Name), zoneName) {
			z := zone
			return &z, nil
		}
	}
	return nil, nil
}

func (c *Client) FindCNAMERecordID(ctx context.Context, zoneID string, fqdn string) (string, error) {
	if zoneID == "" || fqdn == "" || c.APIToken == "" {
		return "", nil
	}
	reqURL := fmt.Sprintf("%s/zones/%s/dns_records?type=CNAME&name=%s", baseURL, zoneID, url.QueryEscape(fqdn))
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, reqURL, nil, &out)
	if err != nil {
		return "", err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return "", fmt.Errorf(firstErrorMessage(raw, "dns lookup failed", msg))
	}
	if len(out.Result) == 0 {
		return "", nil
	}
	return out.Result[0].ID, nil
}

func (c *Client) FindDNSRecords(ctx context.Context, zoneID string, fqdn string, recordType string) ([]DNSRecord, error) {
	if zoneID == "" || fqdn == "" || c.APIToken == "" {
		return []DNSRecord{}, nil
	}
	values := url.Values{}
	values.Set("name", fqdn)
	if strings.TrimSpace(recordType) != "" {
		values.Set("type", strings.ToUpper(strings.TrimSpace(recordType)))
	}

	var out struct {
		Success bool        `json:"success"`
		Result  []DNSRecord `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/zones/%s/dns_records?%s", baseURL, zoneID, values.Encode()), nil, &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return nil, fmt.Errorf(firstErrorMessage(raw, "dns lookup failed", msg))
	}
	if out.Result == nil {
		return []DNSRecord{}, nil
	}
	return out.Result, nil
}

func (c *Client) FindDNSRecord(ctx context.Context, zoneID string, fqdn string, recordType string) (*DNSRecord, error) {
	records, err := c.FindDNSRecords(ctx, zoneID, fqdn, recordType)
	if err != nil || len(records) == 0 {
		return nil, err
	}
	record := records[0]
	return &record, nil
}

func (c *Client) CreateDNSRecord(ctx context.Context, zoneID string, input DNSRecordInput) (DNSRecord, error) {
	body := map[string]any{
		"type":    input.Type,
		"name":    input.Name,
		"content": input.Content,
	}
	if input.Proxied != nil {
		body["proxied"] = *input.Proxied
	}
	if input.TTL != nil {
		body["ttl"] = *input.TTL
	}

	var out struct {
		Success bool      `json:"success"`
		Result  DNSRecord `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/zones/%s/dns_records", baseURL, zoneID), body, &out)
	if err != nil {
		return DNSRecord{}, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return DNSRecord{}, fmt.Errorf(firstErrorMessage(raw, "DNS record create failed", msg))
	}
	return out.Result, nil
}

func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID string, recordID string, input DNSRecordInput) (DNSRecord, error) {
	body := map[string]any{
		"type":    input.Type,
		"name":    input.Name,
		"content": input.Content,
	}
	if input.Proxied != nil {
		body["proxied"] = *input.Proxied
	}
	if input.TTL != nil {
		body["ttl"] = *input.TTL
	}

	var out struct {
		Success bool      `json:"success"`
		Result  DNSRecord `json:"result"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/zones/%s/dns_records/%s", baseURL, zoneID, recordID), body, &out)
	if err != nil {
		return DNSRecord{}, err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return DNSRecord{}, fmt.Errorf(firstErrorMessage(raw, "DNS record update failed", msg))
	}
	return out.Result, nil
}

func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID string, recordID string) error {
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_, raw, err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("%s/zones/%s/dns_records/%s", baseURL, zoneID, recordID), nil, &out)
	if err != nil {
		return err
	}
	if !out.Success {
		msg := ""
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return fmt.Errorf(firstErrorMessage(raw, "DNS record delete failed", msg))
	}
	return nil
}

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
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
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

func normalizeIngressHostname(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimSuffix(h, ".")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSpace(h)
}
