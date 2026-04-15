package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── Pelican API types ──────────────────────────────────────────────────────

type createServerPayload struct {
	ExternalID        string            `json:"external_id"`
	Name              string            `json:"name"`
	User              int               `json:"user"`
	Egg               int               `json:"egg"`
	Environment       map[string]string `json:"environment"`
	Limits            resourceLimits    `json:"limits"`
	FeatureLimits     featureLimits     `json:"feature_limits"`
	Deploy            deployConfig      `json:"deploy"`
	StartOnCompletion bool              `json:"start_on_completion"`
	DockerImage       string            `json:"docker_image,omitempty"`
	Startup           string            `json:"startup,omitempty"`
}

type deployConfig struct {
	Locations   []int    `json:"locations"`
	Tags        []string `json:"tags"`
	DedicatedIP bool     `json:"dedicated_ip"`
	PortRange   []int    `json:"port_range"`
}

type applicationEnvelope[T any] struct {
	Attributes T `json:"attributes"`
}

type applicationServer struct {
	ID         int    `json:"id"`
	ExternalID string `json:"external_id"`
	UUID       string `json:"uuid"`
	Identifier string `json:"identifier"`
	Node       int    `json:"node"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updated_at"`
	Container  struct {
		Installed flexibleBool `json:"installed"`
	} `json:"container"`
}

type flexibleBool bool

func (b *flexibleBool) UnmarshalJSON(data []byte) error {
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case "true", "1":
		*b = true
		return nil
	case "false", "0", "null":
		*b = false
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("invalid bool value %q", string(data))
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		*b = true
	case "false", "0", "no", "":
		*b = false
	default:
		return fmt.Errorf("invalid bool value %q", value)
	}
	return nil
}

func (b flexibleBool) Bool() bool { return bool(b) }

type applicationNode struct {
	ID            int    `json:"id"`
	FQDN          string `json:"fqdn"`
	Scheme        string `json:"scheme"`
	DaemonConnect int    `json:"daemon_connect"`
}

type nodeConfiguration struct {
	Token string `json:"token"`
}

type nodeEndpoint struct {
	BaseURL string
	Token   string
}

type daemonServer struct {
	State string `json:"state"`
}

// ── Pelican Application API ────────────────────────────────────────────────

func (c *controller) createServer(ctx context.Context, playerName string, tpl templateConfig) (applicationServer, error) {
	timestamp := time.Now().UTC().Format("20060102-150405")
	suffix := randomSuffix(3)
	serverName := fmt.Sprintf("%s-%s-%s-%s",
		sanitizeName(tpl.NamePrefix), sanitizeName(playerName), timestamp, suffix)

	adminName := tpl.AdminName
	if adminName == "" {
		adminName = playerName
	}

	payload := createServerPayload{
		ExternalID: serverName,
		Name:       serverName,
		User:       tpl.UserID,
		Egg:        tpl.EggID,
		Environment: map[string]string{
			"LUANTI_SERVER_KIND":      "instance",
			"INSTANCE_TEMPLATE_NAME":  tpl.TemplateName,
			"INSTANCE_TEMPLATE_MOUNT": tpl.InstanceTemplateMount,
			"SERVER_LIST_URL":         tpl.ServerListURL,
			"COMMUNITY_DOWNLOAD":      "0",
			"COMMUNITY_GAME_AUTOR":    "",
			"COMMUNITY_GAME_NAME":     "",
			"SERVER_DESC":             tpl.ServerDescription,
			"SERVER_DOMAIN":           tpl.ServerDomain,
			"DEFAULT_GAME":            c.cfg.DefaultGame,
			"MINETEST_GAME_PATH":      "/home/container/.luanti/games",
			"SERVER_MAX_USERS":        tpl.ServerMaxUsers,
			"SERVER_MOTD":             tpl.ServerMOTD,
			"SERVER_ADMIN_NAME":       adminName,
			"SERVER_NAME":             serverName,
			"SERVER_PASSWORD":         tpl.ServerPassword,
			"SERVER_URL":              tpl.ServerURL,
			"SERVER_ANNOUNCE":         fmt.Sprintf("%t", tpl.ServerAnnounce),
			"WORLD_NAME":              tpl.WorldName,
			"SERVER_PORT":             fmt.Sprintf("%d", tpl.InternalPort),
			"BIND_ADDR":               "0.0.0.0",
		},
		Limits:        tpl.Limits,
		FeatureLimits: tpl.FeatureLimits,
		Deploy: deployConfig{
			Locations:   tpl.LocationIDs,
			Tags:        tpl.Tags,
			DedicatedIP: false,
			PortRange:   []int{},
		},
		StartOnCompletion: false,
		DockerImage:       tpl.DockerImage,
		Startup:           tpl.Startup,
	}

	var resp applicationEnvelope[applicationServer]
	if err := c.panelRequest(ctx, http.MethodPost, "/api/application/servers", payload, &resp); err != nil {
		return applicationServer{}, fmt.Errorf("create server: %w", err)
	}
	return resp.Attributes, nil
}

func (c *controller) fetchServer(ctx context.Context, serverID int) (applicationServer, error) {
	var resp applicationEnvelope[applicationServer]
	if err := c.panelRequest(ctx, http.MethodGet,
		fmt.Sprintf("/api/application/servers/%d", serverID), nil, &resp); err != nil {
		return applicationServer{}, fmt.Errorf("fetch server %d: %w", serverID, err)
	}
	return resp.Attributes, nil
}

func (c *controller) attachMount(ctx context.Context, mountID, serverID int) error {
	body := map[string][]int{"servers": {serverID}}
	if err := c.panelRequest(ctx, http.MethodPost,
		fmt.Sprintf("/api/application/mounts/%d/servers", mountID), body, nil); err != nil {
		return fmt.Errorf("attach mount %d: %w", mountID, err)
	}
	return nil
}

func (c *controller) reinstallServer(ctx context.Context, serverID int) error {
	if err := c.panelRequest(ctx, http.MethodPost,
		fmt.Sprintf("/api/application/servers/%d/reinstall", serverID), nil, nil); err != nil {
		return fmt.Errorf("reinstall server %d: %w", serverID, err)
	}
	return nil
}

func (c *controller) deleteServer(ctx context.Context, serverID int) error {
	if serverID == 0 {
		return nil
	}
	if err := c.panelRequest(ctx, http.MethodDelete,
		fmt.Sprintf("/api/application/servers/%d/force", serverID), nil, nil); err != nil {
		return fmt.Errorf("delete server %d: %w", serverID, err)
	}
	return nil
}

func (c *controller) waitForInstall(ctx context.Context, serverID int, baselineUpdatedAt, phase string) (applicationServer, error) {
	ticker := time.NewTicker(time.Duration(c.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return applicationServer{}, fmt.Errorf("wait for %s: %w", phase, ctx.Err())
		case <-ticker.C:
			server, err := c.fetchServer(ctx, serverID)
			if err != nil {
				return applicationServer{}, err
			}
			if isInstallComplete(server) && (baselineUpdatedAt == "" || server.UpdatedAt != baselineUpdatedAt) {
				return server, nil
			}
		}
	}
}

func isInstallComplete(server applicationServer) bool {
	return server.Container.Installed.Bool() && !strings.EqualFold(server.Status, "installing")
}

// ── Wings Daemon API ───────────────────────────────────────────────────────

func (c *controller) nodeEndpointFor(ctx context.Context, nodeID int) (nodeEndpoint, error) {
	c.nodeMu.Lock()
	if ep, ok := c.nodeCache[nodeID]; ok {
		c.nodeMu.Unlock()
		return ep, nil
	}
	c.nodeMu.Unlock()

	var nodeResp applicationEnvelope[applicationNode]
	if err := c.panelRequest(ctx, http.MethodGet,
		fmt.Sprintf("/api/application/nodes/%d", nodeID), nil, &nodeResp); err != nil {
		return nodeEndpoint{}, fmt.Errorf("fetch node %d: %w", nodeID, err)
	}
	var cfgResp nodeConfiguration
	if err := c.panelRequest(ctx, http.MethodGet,
		fmt.Sprintf("/api/application/nodes/%d/configuration", nodeID), nil, &cfgResp); err != nil {
		return nodeEndpoint{}, fmt.Errorf("fetch node %d configuration: %w", nodeID, err)
	}

	if nodeResp.Attributes.FQDN == "" || nodeResp.Attributes.Scheme == "" || nodeResp.Attributes.DaemonConnect == 0 {
		return nodeEndpoint{}, fmt.Errorf("node %d is missing daemon connection details", nodeID)
	}
	if cfgResp.Token == "" {
		return nodeEndpoint{}, fmt.Errorf("node %d returned an empty daemon token", nodeID)
	}

	ep := nodeEndpoint{
		BaseURL: fmt.Sprintf("%s://%s:%d",
			nodeResp.Attributes.Scheme, nodeResp.Attributes.FQDN, nodeResp.Attributes.DaemonConnect),
		Token: cfgResp.Token,
	}

	c.nodeMu.Lock()
	c.nodeCache[nodeID] = ep
	c.nodeMu.Unlock()
	return ep, nil
}

func (c *controller) startDaemonServer(ctx context.Context, node nodeEndpoint, serverUUID string) error {
	body := map[string]string{"action": "start"}
	if err := c.daemonRequest(ctx, node, http.MethodPost,
		fmt.Sprintf("/api/servers/%s/power", serverUUID), body, nil); err != nil {
		return fmt.Errorf("start daemon server %s: %w", serverUUID, err)
	}
	return nil
}

func (c *controller) stopDaemonServer(ctx context.Context, node nodeEndpoint, serverUUID string) error {
	body := map[string]string{"action": "stop"}
	if err := c.daemonRequest(ctx, node, http.MethodPost,
		fmt.Sprintf("/api/servers/%s/power", serverUUID), body, nil); err != nil {
		return fmt.Errorf("stop daemon server %s: %w", serverUUID, err)
	}
	return nil
}

func (c *controller) killDaemonServer(ctx context.Context, node nodeEndpoint, serverUUID string) error {
	body := map[string]string{"action": "kill"}
	if err := c.daemonRequest(ctx, node, http.MethodPost,
		fmt.Sprintf("/api/servers/%s/power", serverUUID), body, nil); err != nil {
		return fmt.Errorf("kill daemon server %s: %w", serverUUID, err)
	}
	return nil
}

func (c *controller) waitForDaemonState(ctx context.Context, node nodeEndpoint, serverUUID, want string) error {
	ticker := time.NewTicker(time.Duration(c.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for daemon state %q: %w", want, ctx.Err())
		case <-ticker.C:
			var resp daemonServer
			if err := c.daemonRequest(ctx, node, http.MethodGet,
				fmt.Sprintf("/api/servers/%s", serverUUID), nil, &resp); err != nil {
				return err
			}
			if strings.EqualFold(resp.State, want) {
				return nil
			}
		}
	}
}

// ── HTTP helpers ───────────────────────────────────────────────────────────

func (c *controller) panelRequest(ctx context.Context, method, path string, body any, out any) error {
	return c.doJSONRequest(ctx, c.cfg.PanelURL+path, c.cfg.ApplicationToken, method, body, out)
}

func (c *controller) daemonRequest(ctx context.Context, node nodeEndpoint, method, path string, body any, out any) error {
	return c.doJSONRequest(ctx, node.BaseURL+path, node.Token, method, body, out)
}

func (c *controller) doJSONRequest(ctx context.Context, url, token, method string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			trimmed = resp.Status
		}
		return fmt.Errorf("%s %s returned %s: %s", method, url, resp.Status, trimmed)
	}

	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// ── Name helpers ───────────────────────────────────────────────────────────

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "srv"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "srv"
	}
	return out
}

func makeProxyName(prefix, player string) string {
	return fmt.Sprintf("%s-%s-%s", sanitizeName(prefix), sanitizeName(player), randomSuffix(2))
}

func randomSuffix(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := crand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	}
	return hex.EncodeToString(buf)
}
