package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Instance Lifecycle ──────────────────────────────────────────────────────

// provisionInstance creates a new Pelican server from a template and registers it with the proxy.
func (c *controller) provisionInstance(classID *int, createdBy, templateName, displayName string) (*instanceData, error) {
	tpl, ok := c.cfg.Templates[templateName]
	if !ok {
		return nil, fmt.Errorf("template %q not found", templateName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.PollTimeoutSeconds)*time.Second)
	defer cancel()

	// 1. Create Pelican server
	appSrv, err := c.createServer(ctx, createdBy, tpl)
	if err != nil {
		return nil, err
	}

	institute, err := c.getTeacherInstitute(createdBy)
	if err != nil {
		return nil, fmt.Errorf("failed to load creator institute: %w", err)
	}

	inst := &instanceData{
		ID:           appSrv.ExternalID,
		ClassID:      classID,
		CreatedBy:    createdBy,
		Institute:    institute,
		DisplayName:  displayName,
		TemplateName: templateName,
		CreatedAt:    time.Now(),
		ServerID:     appSrv.ID,
		UUID:         appSrv.UUID,
		NodeID:       appSrv.Node,
		ProxyName:    makeProxyName(tpl.NamePrefix, createdBy),
		BackendAddr:  fmt.Sprintf("%s:%d", appSrv.UUID, c.cfg.Instance.InternalPort),
		Status:       "provisioning",
	}

	// Save initial record so we don't lose track of it if we crash
	if err := c.createInstance(inst); err != nil {
		// Try to cleanup the Pelican server if DB insert fails
		_ = c.deleteServer(context.Background(), appSrv.ID)
		return nil, fmt.Errorf("failed to save instance record: %w", err)
	}

	// 2. Wait for initial install
	appSrv, err = c.waitForInstall(ctx, appSrv.ID, "", "initial install")
	if err != nil {
		return inst, err
	}

	// 3. Attach mounts
	mountIDs := c.cfg.Instance.MountIDs
	if len(mountIDs) == 0 && c.cfg.Instance.MountID != 0 {
		mountIDs = []int{c.cfg.Instance.MountID}
	}
	for _, mid := range mountIDs {
		if err := c.attachMount(ctx, mid, appSrv.ID); err != nil {
			return inst, err
		}
	}

	// 4. Reinstall to apply mount
	baselineUpdatedAt := appSrv.UpdatedAt
	if err := c.reinstallServer(ctx, appSrv.ID); err != nil {
		return inst, err
	}

	// 5. Wait for reinstall
	appSrv, err = c.waitForInstall(ctx, appSrv.ID, baselineUpdatedAt, "reinstall")
	if err != nil {
		return inst, err
	}

	// 6. Start the server
	if err := c.startInstance(inst); err != nil {
		return inst, err
	}

	return inst, nil
}

// startInstance starts a provisioned server via Wings and registers it with the proxy.
func (c *controller) startInstance(inst *instanceData) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.PollTimeoutSeconds)*time.Second)
	defer cancel()

	node, err := c.nodeEndpointFor(ctx, inst.NodeID)
	if err != nil {
		return err
	}

	if err := c.startDaemonServer(ctx, node, inst.UUID); err != nil {
		return err
	}

	if err := c.waitForDaemonState(ctx, node, inst.UUID, "running"); err != nil {
		return err
	}

	// Add a grace period for the Luanti process to actually start listening
	if c.cfg.StartGraceSeconds > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(c.cfg.StartGraceSeconds) * time.Second):
		}
	}

	// Register with proxy
	if ok := proxy.AddServer(inst.ProxyName, proxy.Server{
		Addr:      inst.BackendAddr,
		MediaPool: c.cfg.Instance.MediaPool,
		Groups:    c.cfg.Instance.Groups,
		Fallback:  c.cfg.LobbyServer,
	}); !ok {
		return fmt.Errorf("proxy refused to register server %s", inst.ProxyName)
	}

	return c.updateInstanceStatus(inst.ID, "running")
}

// evacuateToLobby hops all players on the given server back to the lobby.
func (c *controller) evacuateToLobby(serverName string) {
	for cc := range proxy.Clts() {
		if cc.ServerName() == serverName {
			if err := cc.Hop(c.cfg.LobbyServer); err != nil {
				log.Printf("[%s] failed to hop %s to lobby: %v", pluginName, cc.Name(), err)
			}
		}
	}
}

// stopInstance stops a running server via Wings and unregisters it from the proxy.
func (c *controller) stopInstance(inst *instanceData) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.PollTimeoutSeconds)*time.Second)
	defer cancel()

	// Move all players on this instance back to the lobby
	c.evacuateToLobby(inst.ProxyName)

	// Unregister from proxy now that no players are on it
	if !proxy.RmServer(inst.ProxyName) {
		log.Printf("[%s] warning: could not remove server %s from proxy", pluginName, inst.ProxyName)
	}

	node, err := c.nodeEndpointFor(ctx, inst.NodeID)
	if err != nil {
		return err
	}

	if err := c.stopDaemonServer(ctx, node, inst.UUID); err != nil {
		return err
	}

	if err := c.waitForDaemonState(ctx, node, inst.UUID, "offline"); err != nil {
		// If stop fails, try kill as fallback
		_ = c.killDaemonServer(ctx, node, inst.UUID)
		_ = c.waitForDaemonState(ctx, node, inst.UUID, "offline")
	}

	return c.updateInstanceStatus(inst.ID, "stopped")
}

// deleteInstance stops, unregisters, and deletes the Pelican server.
func (c *controller) deleteInstance(inst *instanceData) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.PollTimeoutSeconds)*time.Second)
	defer cancel()

	// 1. Evacuate players and stop if running
	c.evacuateToLobby(inst.ProxyName)
	if inst.Status == "running" {
		_ = c.stopInstance(inst)
	} else {
		proxy.RmServer(inst.ProxyName)
	}

	// 2. Delete from Pelican
	if err := c.deleteServer(ctx, inst.ServerID); err != nil {
		log.Printf("[%s] warning: failed to delete Pelican server %d: %v", pluginName, inst.ServerID, err)
	}

	// 3. Mark as deleted in DB
	return c.deleteInstanceRecord(inst.ID)
}

// ── Startup Reconciliation ──────────────────────────────────────────────────

// reconcileInstances finds all "running" instances in DB and ensures they are in proxy.
func (c *controller) reconcileInstances() {
	instances, err := c.getAllActiveInstances()
	if err != nil {
		log.Printf("[%s] reconciliation error: %v", pluginName, err)
		return
	}

	count := 0
	for i := range instances {
		inst := &instances[i]
		if inst.Status != "running" {
			continue
		}

		_, ok := c.cfg.Templates[inst.TemplateName]
		if !ok {
			log.Printf("[%s] reconciliation: template %q for instance %s no longer exists", pluginName, inst.TemplateName, inst.ID)
			continue
		}

		// Re-register with proxy
		if ok := proxy.AddServer(inst.ProxyName, proxy.Server{
			Addr:      inst.BackendAddr,
			MediaPool: c.cfg.Instance.MediaPool,
			Groups:    c.cfg.Instance.Groups,
			Fallback:  c.cfg.LobbyServer,
		}); ok {
			count++
		}
	}

	if count > 0 {
		log.Printf("[%s] reconciled %d running instances with proxy", pluginName, count)
	}
}

// ── Background Status Check ────────────────────────────────────────────────

// startStatusChecker starts a goroutine that periodically checks Pelican/Wings
// status for all "running" instances and updates the DB if they went offline.
func (c *controller) startStatusChecker() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c.checkAllInstancesStatus()
		}
	}()
}

func (c *controller) checkAllInstancesStatus() {
	instances, err := c.getAllActiveInstances()
	if err != nil {
		return
	}

	for i := range instances {
		inst := &instances[i]
		if inst.Status != "running" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		node, err := c.nodeEndpointFor(ctx, inst.NodeID)
		if err != nil {
			cancel()
			continue
		}

		var resp daemonServer
		if err := c.daemonRequest(ctx, node, "GET", fmt.Sprintf("/api/servers/%s", inst.UUID), nil, &resp); err != nil {
			cancel()
			continue
		}
		cancel()

		if !strings.EqualFold(resp.State, "running") && !strings.EqualFold(resp.State, "starting") {
			log.Printf("[%s] instance %s went %s, updating status", pluginName, inst.ID, resp.State)
			_ = c.updateInstanceStatus(inst.ID, "stopped")
			proxy.RmServer(inst.ProxyName)
		}
	}
}
