package main

import (
	"log"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Administrators ──────────────────────────────────────────────────────────
//
// Administrators come from two places: the proxy configuration (the "server"
// permission, which the proxy egg writes from PROXY_ADMIN_USERS on every
// boot) and the panel_admins table, written from the Users tab. Only the
// second kind can be granted or removed from the panel. Panel administrators
// have every right of the plugin, but not the proxy's own commands.

// loadPanelAdmins reads panel_admins into memory; isAdmin is called often.
func (c *controller) loadPanelAdmins() error {
	names, err := c.queryStrings("SELECT username FROM panel_admins")
	if err != nil {
		return err
	}
	admins := make(map[string]bool, len(names))
	for _, n := range names {
		admins[n] = true
	}
	c.adminMu.Lock()
	c.panelAdmins = admins
	c.adminMu.Unlock()
	return nil
}

func (c *controller) isPanelAdmin(name string) bool {
	c.adminMu.RLock()
	defer c.adminMu.RUnlock()
	return c.panelAdmins[name]
}

// grantPanelAdmin makes name an administrator. Their other roles are kept.
func (c *controller) grantPanelAdmin(actor, name string) (bool, string) {
	if !accountNamePattern.MatchString(name) {
		return false, T("Invalid player name.")
	}
	if isProxyAdmin(name) || c.isPanelAdmin(name) {
		return false, T("@1 is already an administrator.", name)
	}
	if _, err := c.db.Exec(`INSERT INTO panel_admins (username, granted_by) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE granted_by = VALUES(granted_by)`, name, actor); err != nil {
		log.Printf("[%s] grant admin %s: %v", pluginName, name, err)
		return false, T("Database error.")
	}
	c.adminMu.Lock()
	c.panelAdmins[name] = true
	c.adminMu.Unlock()
	log.Printf("[%s] %s made %s an administrator", pluginName, actor, name)
	if proxy.Find(name) != nil {
		c.scheduleReapplyStates(name, 0)
	}
	return true, T("@1 is now: @2.", name, roleLabel(roleAdmin))
}

// forgetPanelAdmin updates the cache after panel_admins lost a row.
func (c *controller) forgetPanelAdmin(actor, name string) {
	c.adminMu.Lock()
	delete(c.panelAdmins, name)
	c.adminMu.Unlock()
	log.Printf("[%s] %s removed the administrator rights of %s", pluginName, actor, name)
}
