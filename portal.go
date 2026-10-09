package main

import (
	"fmt"
	"sort"
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

// classStaffInWorld reports whether someone of the class staff (owner,
// teachers, Assistance) is currently inside the world.
func (c *controller) classStaffInWorld(inst *instanceData) bool {
	if inst == nil || inst.ClassID == nil {
		return false
	}
	staff, err := c.getClassStaff(*inst.ClassID)
	if err != nil {
		return false
	}
	for _, name := range staff {
		if p := proxy.Find(name); p != nil && p.ServerName() == inst.ProxyName {
			return true
		}
	}
	return false
}

// portalRole is how a player enters a world from the HUB portal.
type portalRole int

const (
	portalSpectator portalRole = iota
	portalStudent              // own class with a teacher inside
	portalStaff                // staff of the class or admin: normal role
)

func (c *controller) portalRoleFor(player string, inst *instanceData, studentClass *classData) portalRole {
	hasClassStaffAccess := inst.ClassID != nil && c.canEditClassStudents(*inst.ClassID, player)
	if !shouldUsePortalSpectator(inst, player, c.isAdmin(player), hasClassStaffAccess) {
		return portalStaff
	}
	if studentClass != nil && inst.ClassID != nil && *inst.ClassID == studentClass.ID && c.classStaffInWorld(inst) {
		return portalStudent
	}
	return portalSpectator
}

func (c *controller) showPortalWorlds(cc *proxy.ClientConn) {
	if cc == nil {
		return
	}

	instances, err := c.getRunningClassInstances()
	if err != nil {
		cc.SendChatMsg("[Classrooms] The active world list is temporarily unavailable.")
		return
	}
	name := cc.Name()
	studentClass, _ := c.getStudentClass(name)

	// The player's own class first; within each part, worlds with a teacher
	// inside come first.
	staffInside := map[string]bool{}
	var mine, others []instanceData
	for _, inst := range instances {
		staffInside[inst.ID] = c.classStaffInWorld(&inst)
		if studentClass != nil && inst.ClassID != nil && *inst.ClassID == studentClass.ID {
			mine = append(mine, inst)
		} else {
			others = append(others, inst)
		}
	}
	byTeacherInside := func(list []instanceData) {
		sort.SliceStable(list, func(i, j int) bool {
			return staffInside[list[i].ID] && !staffInside[list[j].ID]
		})
	}
	byTeacherInside(mine)
	byTeacherInside(others)

	var b strings.Builder
	fsOpen(&b, 12.6, 10)
	fsHeader(&b, 12.6, "Class worlds", "Live class worlds you can enter", false, true)

	if len(instances) == 0 {
		b.WriteString(box(0.3, 1.3, 12, 2.0, colorCard))
		b.WriteString(fmt.Sprintf("image[0.6,1.65;0.55,0.55;%s]", iconGlobe))
		b.WriteString(coloredLbl(1.35, 1.95, light, "No class worlds are open right now."))
		b.WriteString(hint(1.35, 2.45, "Come back when your teacher opens one."))
		cc.ShowFormspec("classrooms:portal_worlds", b.String())
		return
	}

	const cardH, gap = 1.3, 0.12
	rows := len(instances) + 1
	if len(mine) > 0 && len(others) > 0 {
		rows++
	}
	listH := 7.2
	b.WriteString(scrollbarFor("scr_portal_worlds", 12.15, 1.3, listH, rows, cardH+gap, 0.05))
	b.WriteString(fmt.Sprintf("scroll_container[0.3,1.3;11.75,%g;scr_portal_worlds;vertical;0.1]", listH))
	y := 0.05
	section := func(title string) {
		b.WriteString(sectionTitle(0.1, y+0.3, title))
		y += 0.6
	}
	card := func(inst instanceData, own bool) {
		role := c.portalRoleFor(name, &inst, studentClass)
		bg := colorCard
		if own {
			bg = colorCurrent
		}
		b.WriteString(box(0, y, 11.65, cardH, bg))
		if own {
			b.WriteString(box(0, y, 0.1, cardH, colorActive))
		}
		b.WriteString(statusDot(0.3, y+0.24, success))
		title := mcColorize(light, inst.Title())
		if staffInside[inst.ID] {
			title += mcColorize(success, "   ● Teacher inside")
		}
		if cc.ServerName() == inst.ProxyName {
			title += mcColorize(warning, "   ● You are here")
		}
		b.WriteString(fmt.Sprintf("label[0.7,%g;%s]", y+0.33, fmtEsc(title)))
		players := len(c.playersOnInstance(inst.ProxyName))
		teacher := []rune(inst.CreatedBy)
		if len(teacher) > 14 {
			teacher = append(teacher[:13], '…')
		}
		details := fmt.Sprintf("Map: %s  ·  Teacher: %s  ·  %s inside", inst.TemplateName, string(teacher),
			plural(players, "player", "players"))
		b.WriteString(fmt.Sprintf("label[0.7,%g;%s]", y+0.7, fmtEsc(mcColorize(muted, details))))
		field := "visit_inst_" + inst.ID
		switch role {
		case portalStudent:
			b.WriteString(fmt.Sprintf("label[0.7,%g;%s]", y+1.05, fmtEsc(mcColorize(success,
				"Your teacher is inside: you join as a student"))))
			b.WriteString(styledBtn(9.35, y+0.3, 2.1, 0.7, field, "Join", colorPrimary))
			b.WriteString(tooltip(field, "Enter as a student of your class"))
		case portalStaff:
			b.WriteString(fmt.Sprintf("label[0.7,%g;%s]", y+1.05, fmtEsc(mcColorize(success,
				"You enter with your usual role"))))
			b.WriteString(styledBtn(9.35, y+0.3, 2.1, 0.7, field, "Join", colorPrimary))
		default:
			text := "Spectator: fly through blocks, no building"
			if own {
				text = "Your teacher is not inside yet: you visit as a spectator"
			}
			b.WriteString(fmt.Sprintf("label[0.7,%g;%s]", y+1.05, fmtEsc(mcColorize(warning, text))))
			b.WriteString(styledBtn(9.35, y+0.3, 2.1, 0.7, field, "Visit", colorButton))
			b.WriteString(tooltip(field, "Look around as a spectator: you can fly and pass through blocks but not build"))
		}
		y += cardH + gap
	}
	if len(mine) > 0 {
		section("Your class · " + studentClass.Name)
		for _, inst := range mine {
			card(inst, true)
		}
	}
	if len(others) > 0 {
		if len(mine) > 0 {
			section("Other class worlds")
		} else {
			section("Open class worlds")
		}
		for _, inst := range others {
			card(inst, false)
		}
	}
	b.WriteString("scroll_container_end[]")

	b.WriteString(box(0.3, 8.7, 12, 1.0, colorCard))
	b.WriteString(fmt.Sprintf("image[0.55,8.92;0.5,0.5;%s]", iconEnter))
	b.WriteString(hint(1.25, 9.05, "Spectators fly and pass through blocks but cannot build."))
	b.WriteString(hint(1.25, 9.45, "Use the door in your hotbar to come back, the map to teleport."))

	cc.ShowFormspec("classrooms:portal_worlds", b.String())
}

func (c *controller) handlePortalWorlds(cc *proxy.ClientConn, fields []mt.Field) {
	if cc == nil {
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

		// Staff keep their role; students of the class join normally while a
		// teacher of the class is inside; everybody else visits as spectator.
		studentClass, _ := c.getStudentClass(cc.Name())
		if c.portalRoleFor(cc.Name(), inst, studentClass) == portalSpectator {
			c.setPortalVisitor(cc.Name(), inst.ID)
		} else {
			c.clearPortalVisitor(cc.Name())
		}
		if cc.ServerName() == inst.ProxyName {
			// Already here: only switch role (e.g. spectator -> student once
			// the teacher arrived).
			c.scheduleReapplyStates(cc.Name(), 0)
			return
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
