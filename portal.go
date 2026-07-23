package main

import (
	"fmt"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// getRunningClassInstances returns only live, class-bound worlds. Standalone
// admin instances are intentionally not advertised by the public HUB portal.
func (c *controller) getRunningClassInstances() ([]instanceData, error) {
	rows, err := c.db.Query(`SELECT id, class_id, created_by, institute, display_name, template_name, created_at,
		server_id, uuid, node_id, proxy_name, backend_addr, status
		FROM instances
		WHERE class_id IS NOT NULL AND status = 'running'
		ORDER BY display_name, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func (c *controller) showPortalWorlds(cc *proxy.ClientConn) {
	if cc == nil || cc.ServerName() != c.cfg.LobbyServer {
		return
	}

	instances, err := c.getRunningClassInstances()
	if err != nil {
		cc.SendChatMsg("[Classrooms] The active world list is temporarily unavailable.")
		return
	}

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[12,8.2]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))
	b.WriteString(box(0, 0, 12, 0.9, panel))
	b.WriteString(coloredLbl(0.4, 0.42, light, "Active World Class Instances"))
	b.WriteString(btnExit(10.35, 0.18, 1.25, 0.52, "btn_close", "Close"))
	b.WriteString(box(0, 0.9, 12, 0.04, accent))
	b.WriteString(coloredLbl(0.4, 1.35, muted,
		"Visit a live class world in spectator mode. You cannot build, damage, or interact."))

	if len(instances) == 0 {
		b.WriteString(coloredLbl(0.55, 2.45, muted, "No World Class instances are active right now."))
	} else {
		b.WriteString(scrollbarFor("scr_portal_worlds", 11.45, 2.0, 5.75, len(instances), 0.95, 0.05))
		b.WriteString("scroll_container[0.3,2.0;11.05,5.75;scr_portal_worlds;vertical;0.1]")
		y := 0.05
		for _, inst := range instances {
			subtitle := "Teacher: " + inst.CreatedBy
			if strings.TrimSpace(inst.Institute) != "" {
				subtitle += "  |  " + inst.Institute
			}
			b.WriteString(box(0, y, 10.9, 0.85, panel))
			b.WriteString(fmt.Sprintf("label[0.25,%g;%s]", y+0.27,
				fmtEsc(mcColorize(light, inst.Title()))))
			b.WriteString(fmt.Sprintf("label[0.25,%g;%s]", y+0.58,
				fmtEsc(mcColorize(muted, subtitle))))
			b.WriteString(fmt.Sprintf("button[9.25,%g;1.25,0.55;visit_inst_%s;Visit]",
				y+0.14, fmtEsc(inst.ID)))
			y += 0.95
		}
		b.WriteString("scroll_container_end[]")
	}

	cc.ShowFormspec("classrooms:portal_worlds", b.String())
}

func (c *controller) handlePortalWorlds(cc *proxy.ClientConn, fields []mt.Field) {
	if cc == nil || cc.ServerName() != c.cfg.LobbyServer {
		return
	}

	fm := fieldMap(fields)
	for name := range fm {
		if !strings.HasPrefix(name, "visit_inst_") {
			continue
		}

		instanceID := strings.TrimPrefix(name, "visit_inst_")
		inst, err := c.getInstanceByID(instanceID)
		if err != nil || inst == nil || inst.ClassID == nil || inst.Status != "running" {
			c.notify(cc, "That class world is no longer available.")
			c.showPortalWorlds(cc)
			return
		}

		// Staff retain their normal role only in a world they already control.
		// Students, invited users, generic visitors, and unrelated teachers all
		// enter through the public portal as spectators.
		hasClassStaffAccess := inst.ClassID != nil &&
			c.canEditClassStudents(*inst.ClassID, cc.Name())
		if shouldUsePortalSpectator(inst, cc.Name(), c.isAdmin(cc.Name()), hasClassStaffAccess) {
			c.setPortalVisitor(cc.Name(), inst.ID)
		}
		if err := c.hopPlayer(cc, inst.ProxyName); err != nil {
			c.clearPortalVisitor(cc.Name())
			c.notify(cc, "Failed to enter that class world: "+err.Error())
		}
		return
	}
}

func shouldUsePortalSpectator(inst *instanceData, player string, isAdmin, hasClassStaffAccess bool) bool {
	if inst == nil {
		return true
	}
	return !isAdmin && inst.CreatedBy != player && !hasClassStaffAccess
}

func (c *controller) setPortalVisitor(player, instanceID string) {
	c.mu.Lock()
	c.runtime.portalVisitors[player] = instanceID
	c.mu.Unlock()
}

func (c *controller) clearPortalVisitor(player string) {
	c.mu.Lock()
	delete(c.runtime.portalVisitors, player)
	c.mu.Unlock()
}

func (c *controller) portalVisitorInstance(player string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.portalVisitors[player]
}

func (c *controller) isCurrentServerClassInstance(cc *proxy.ClientConn) bool {
	if cc == nil {
		return false
	}
	inst, err := c.getInstanceByProxyName(cc.ServerName())
	return err == nil && inst != nil && inst.ClassID != nil
}

func (c *controller) reapplyPortalVisitorContext(playerName string) {
	cc := proxy.Find(playerName)
	if cc == nil || cc.ServerName() == "" {
		return
	}

	instanceID := c.portalVisitorInstance(playerName)
	if instanceID == "" {
		c.sendToPlayerServer(playerName, map[string]string{
			"action": "clear_visitor_defaults",
			"player": playerName,
		})
		return
	}

	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil || inst.Status != "running" || inst.ProxyName != cc.ServerName() {
		c.clearPortalVisitor(playerName)
		c.sendToPlayerServer(playerName, map[string]string{
			"action": "clear_visitor_defaults",
			"player": playerName,
		})
		return
	}

	c.sendToPlayerServer(playerName, map[string]string{
		"action": "set_visitor_defaults",
		"player": playerName,
	})
}
