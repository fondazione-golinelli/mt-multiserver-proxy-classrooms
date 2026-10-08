package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Class groups ────────────────────────────────────────────────────────────
//
// Groups split a class into teams. A student belongs to at most one group per
// class; membership rows reference class_students, so removing a student
// from the class also removes them from their group.

const (
	maxGroupsPerClass = 12
	maxZonesPerWorld  = 40
	zoneMaxSide       = 512
)

const (
	zoneTeachersColor = "#9aa3b5"
	zoneOpenColor     = "#7fd18b"
)

var groupPalette = []string{"#e05252", "#4f8fe0", "#3fb56b", "#e0b43f", "#a35ce0", "#3fc8c8", "#e07a3f", "#d95fa6"}

type classGroup struct {
	ID      int
	ClassID int
	Name    string
	Color   string
	Members []string
}

type zoneData struct {
	ID         int
	InstanceID string
	Name       string
	GroupID    sql.NullInt64
	MinX, MinZ int
	MaxX, MaxZ int
	// Open zones let every student build (no protection), but keep the
	// border, the entry notice and the teleport point.
	Open bool
	// Teleport point; older zones without one cannot be teleported to.
	TPX, TPY, TPZ, TPYaw sql.NullFloat64
}

// zoneAccess is who may build inside a zone.
type zoneAccess struct {
	Open    bool
	GroupID int // 0 = no group
}

// zoneEdit is a zone being drawn with the in-world editor.
type zoneEdit struct {
	InstanceID string
	Name       string
	Access     zoneAccess
}

// zoneAccessOptions lists the "who can build" dropdown entries; indexes are
// 1-based as returned by the dropdown.
func zoneAccessOptions(groups []classGroup) []string {
	options := []string{"Teachers only", "Everyone"}
	for _, g := range groups {
		options = append(options, "Group: "+g.Name)
	}
	return options
}

func zoneAccessFromIndex(idx int, groups []classGroup) (zoneAccess, string, bool) {
	switch {
	case idx == 1:
		return zoneAccess{}, "Teachers only", true
	case idx == 2:
		return zoneAccess{Open: true}, "Everyone", true
	case idx >= 3 && idx-3 < len(groups):
		g := groups[idx-3]
		return zoneAccess{GroupID: g.ID}, "Group: " + g.Name, true
	}
	return zoneAccess{}, "", false
}

func zoneAccessIndex(z zoneData, groups []classGroup) int {
	if z.Open {
		return 2
	}
	if z.GroupID.Valid {
		for i, g := range groups {
			if int64(g.ID) == z.GroupID.Int64 {
				return i + 3
			}
		}
	}
	return 1
}

func (z zoneData) size() string {
	return fmt.Sprintf("%d × %d", z.MaxX-z.MinX+1, z.MaxZ-z.MinZ+1)
}

func cleanShortName(name string, max int) string {
	name = strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", ";", " ", ",", " ").Replace(name))
	for utf8.RuneCountInString(name) > max {
		r := []rune(name)
		name = string(r[:max])
	}
	return name
}

func (c *controller) getGroups(classID int) ([]classGroup, error) {
	rows, err := c.db.Query(
		"SELECT id, class_id, name, color FROM class_groups WHERE class_id = ? ORDER BY id", classID)
	if err != nil {
		return nil, err
	}
	var groups []classGroup
	index := map[int]int{}
	for rows.Next() {
		var g classGroup
		if err := rows.Scan(&g.ID, &g.ClassID, &g.Name, &g.Color); err != nil {
			rows.Close()
			return nil, err
		}
		index[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()

	members, err := c.db.Query(
		"SELECT group_id, username FROM class_group_members WHERE class_id = ? ORDER BY username", classID)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var groupID int
		var name string
		if err := members.Scan(&groupID, &name); err != nil {
			return nil, err
		}
		if i, ok := index[groupID]; ok {
			groups[i].Members = append(groups[i].Members, name)
		}
	}
	return groups, nil
}

// groupByStudent maps each grouped student to their group.
func groupByStudent(groups []classGroup) map[string]classGroup {
	m := map[string]classGroup{}
	for _, g := range groups {
		for _, name := range g.Members {
			m[name] = g
		}
	}
	return m
}

func (c *controller) getGroup(classID, groupID int) (*classGroup, error) {
	groups, err := c.getGroups(classID)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.ID == groupID {
			return &g, nil
		}
	}
	return nil, nil
}

func (c *controller) createGroup(classID int, name string) (bool, string) {
	name = cleanShortName(name, 30)
	if name == "" {
		return false, "Type a name for the group."
	}
	groups, err := c.getGroups(classID)
	if err != nil {
		return false, "Could not load groups."
	}
	if len(groups) >= maxGroupsPerClass {
		return false, fmt.Sprintf("A class can have at most %d groups.", maxGroupsPerClass)
	}
	used := map[string]bool{}
	for _, g := range groups {
		if strings.EqualFold(g.Name, name) {
			return false, "A group with that name already exists."
		}
		used[g.Color] = true
	}
	color := groupPalette[len(groups)%len(groupPalette)]
	for _, candidate := range groupPalette {
		if !used[candidate] {
			color = candidate
			break
		}
	}
	if _, err := c.db.Exec("INSERT INTO class_groups (class_id, name, color) VALUES (?, ?, ?)",
		classID, name, color); err != nil {
		return false, "Could not create the group."
	}
	return true, "Group " + name + " created."
}

func (c *controller) deleteGroup(classID, groupID int) {
	if _, err := c.db.Exec("DELETE FROM class_groups WHERE id = ? AND class_id = ?", groupID, classID); err != nil {
		log.Printf("[%s] delete group %d: %v", pluginName, groupID, err)
	}
	c.pushZonesForClass(classID)
}

// setStudentGroup moves a student into a group; groupID 0 removes them from
// any group.
func (c *controller) setStudentGroup(classID int, username string, groupID int) error {
	if !c.isStudentInClass(classID, username) {
		return fmt.Errorf("%s is not in this class", username)
	}
	if _, err := c.db.Exec("DELETE FROM class_group_members WHERE class_id = ? AND username = ?",
		classID, username); err != nil {
		return err
	}
	if groupID == 0 {
		return nil
	}
	_, err := c.db.Exec(`INSERT INTO class_group_members (group_id, class_id, username)
		SELECT id, class_id, ? FROM class_groups WHERE id = ? AND class_id = ?`, username, groupID, classID)
	return err
}

func onlineOf(names []string) []string {
	var online []string
	for _, n := range names {
		if proxy.Find(n) != nil {
			online = append(online, n)
		}
	}
	return online
}

func (c *controller) isGroupFrozen(g classGroup) bool {
	for _, name := range onlineOf(g.Members) {
		if c.isFrozen(name) {
			return true
		}
	}
	return false
}

func (c *controller) toggleGroupFreeze(g classGroup) {
	frozen := c.isGroupFrozen(g)
	for _, name := range onlineOf(g.Members) {
		if frozen {
			c.unfreezePlayer(name)
		} else {
			c.freezePlayer(name)
		}
	}
}

// ── Protected zones ─────────────────────────────────────────────────────────
//
// Zones belong to a world and cover the full height between two X/Z corners.
// A zone without a group is "teachers only"; a group zone can be edited only
// by that group's members. Class staff can always build. The proxy is the
// source of truth and pushes zones to the classrooms_bridge mod, which
// persists them and enforces them through minetest.is_protected.

func (c *controller) getZones(instanceID string) ([]zoneData, error) {
	rows, err := c.db.Query(`SELECT id, instance_id, name, group_id, min_x, min_z, max_x, max_z,
		open_access, tp_x, tp_y, tp_z, tp_yaw
		FROM instance_zones WHERE instance_id = ? ORDER BY id`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var zones []zoneData
	for rows.Next() {
		var z zoneData
		if err := rows.Scan(&z.ID, &z.InstanceID, &z.Name, &z.GroupID, &z.MinX, &z.MinZ, &z.MaxX, &z.MaxZ,
			&z.Open, &z.TPX, &z.TPY, &z.TPZ, &z.TPYaw); err != nil {
			return nil, err
		}
		zones = append(zones, z)
	}
	return zones, nil
}

// validateZoneAccess checks that a group still exists and returns the value
// for the group_id column.
func (c *controller) validateZoneAccess(inst *instanceData, access zoneAccess) (any, string) {
	if access.GroupID == 0 {
		return nil, ""
	}
	if inst.ClassID == nil {
		return nil, "Group zones need a class world."
	}
	g, err := c.getGroup(*inst.ClassID, access.GroupID)
	if err != nil || g == nil {
		return nil, "That group no longer exists."
	}
	return access.GroupID, ""
}

func (c *controller) updateZoneAccess(inst *instanceData, zoneID int, access zoneAccess) string {
	group, msg := c.validateZoneAccess(inst, access)
	if msg != "" {
		return msg
	}
	if _, err := c.db.Exec("UPDATE instance_zones SET open_access = ?, group_id = ? WHERE id = ? AND instance_id = ?",
		access.Open, group, zoneID, inst.ID); err != nil {
		return "Could not change the zone."
	}
	c.pushZones(inst)
	return ""
}

func (c *controller) createZone(inst *instanceData, name string, access zoneAccess, a, b, tp [3]float64, yaw float64) (bool, string) {
	zones, err := c.getZones(inst.ID)
	if err != nil {
		return false, "Could not load zones."
	}
	name = cleanShortName(name, 30)
	if name == "" {
		name = fmt.Sprintf("Zone %d", len(zones)+1)
	}
	if len(zones) >= maxZonesPerWorld {
		return false, fmt.Sprintf("A world can have at most %d zones.", maxZonesPerWorld)
	}
	round := func(v float64) int {
		if v < 0 {
			return int(v - 0.5)
		}
		return int(v + 0.5)
	}
	minX, maxX := round(a[0]), round(b[0])
	minZ, maxZ := round(a[2]), round(b[2])
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minZ > maxZ {
		minZ, maxZ = maxZ, minZ
	}
	if maxX-minX+1 > zoneMaxSide || maxZ-minZ+1 > zoneMaxSide {
		return false, fmt.Sprintf("A zone can be at most %d blocks per side.", zoneMaxSide)
	}
	group, msg := c.validateZoneAccess(inst, access)
	if msg != "" {
		return false, msg
	}
	if _, err := c.db.Exec(`INSERT INTO instance_zones
		(instance_id, name, group_id, open_access, min_x, min_z, max_x, max_z, tp_x, tp_y, tp_z, tp_yaw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, name, group, access.Open, minX, minZ, maxX, maxZ, tp[0], tp[1], tp[2], yaw); err != nil {
		return false, "Could not save the zone."
	}
	c.pushZones(inst)
	return true, "Zone " + name + " created."
}

func (c *controller) deleteZone(inst *instanceData, zoneID int) {
	if _, err := c.db.Exec("DELETE FROM instance_zones WHERE id = ? AND instance_id = ?", zoneID, inst.ID); err != nil {
		log.Printf("[%s] delete zone %d: %v", pluginName, zoneID, err)
	}
	c.pushZones(inst)
}

// zonesMessage builds the set_zones payload for the bridge mod.
func (c *controller) zonesMessage(inst *instanceData) (map[string]interface{}, error) {
	zones, err := c.getZones(inst.ID)
	if err != nil {
		return nil, err
	}
	groups := map[int]classGroup{}
	var classStudents []string
	if inst.ClassID != nil {
		list, err := c.getGroups(*inst.ClassID)
		if err != nil {
			return nil, err
		}
		for _, g := range list {
			groups[g.ID] = g
		}
		classStudents, _ = c.getStudents(*inst.ClassID)
	}
	missionsByZone, err := c.getMissionsForWorld(inst.ID)
	if err != nil {
		return nil, err
	}
	payload := make([]map[string]interface{}, 0, len(zones))
	for _, z := range zones {
		entry := map[string]interface{}{
			"id":    z.ID,
			"name":  z.Name,
			"min_x": z.MinX, "min_z": z.MinZ,
			"max_x": z.MaxX, "max_z": z.MaxZ,
			"color": "#9aa3b5",
		}
		if z.Open {
			entry["open"] = true
			entry["color"] = zoneOpenColor
		} else if z.GroupID.Valid {
			g, ok := groups[int(z.GroupID.Int64)]
			if !ok {
				continue
			}
			entry["group"] = g.Name
			entry["color"] = g.Color
			entry["allowed"] = append([]string{}, g.Members...)
		}
		if _, y, _, _, ok := z.teleportPoint(); ok {
			entry["ref_y"] = y
		}
		if m := missionsByZone[z.ID]; m != nil {
			// Group zones: the group plays the mission; otherwise the class.
			participants := classStudents
			if g, ok := groups[int(z.GroupID.Int64)]; z.GroupID.Valid && ok && !z.Open {
				participants = g.Members
			}
			entry["mission"] = missionPayload(m, participants)
		}
		payload = append(payload, entry)
	}
	return map[string]interface{}{"action": "set_zones", "zones": payload}, nil
}

// pushZones sends the world's zones through any player currently on it.
// Players joining later receive them from reapplyStates.
func (c *controller) pushZones(inst *instanceData) {
	if inst == nil {
		return
	}
	msg, err := c.zonesMessage(inst)
	if err != nil {
		log.Printf("[%s] build zones for %s: %v", pluginName, inst.ID, err)
		return
	}
	for cc := range proxy.Clts() {
		if cc.ServerName() == inst.ProxyName && c.sendToPlayerServer(cc.Name(), msg) {
			return
		}
	}
}

func (c *controller) pushZonesForClass(classID int) {
	instances, err := c.getInstancesForClass(classID)
	if err != nil {
		return
	}
	for i := range instances {
		if instances[i].Status == "running" {
			c.pushZones(&instances[i])
		}
	}
}

// reapplyZones refreshes the zones of the world a player just joined.
func (c *controller) reapplyZones(playerName string) {
	cc := proxy.Find(playerName)
	if cc == nil || cc.ServerName() == "" || cc.ServerName() == c.cfg.LobbyServer {
		return
	}
	inst, err := c.getInstanceByProxyName(cc.ServerName())
	if err != nil || inst == nil {
		return
	}
	msg, err := c.zonesMessage(inst)
	if err != nil {
		log.Printf("[%s] build zones for %s: %v", pluginName, inst.ID, err)
		return
	}
	c.sendToPlayerServer(playerName, msg)
}

// zoneTeleportPoint returns where players land for a zone.
func (z zoneData) teleportPoint() (x, y, z2, yaw float64, ok bool) {
	if z.TPX.Valid && z.TPY.Valid && z.TPZ.Valid {
		return z.TPX.Float64, z.TPY.Float64, z.TPZ.Float64, z.TPYaw.Float64, true
	}
	return 0, 0, 0, 0, false
}

// teleportToZone moves players to a zone's teleport point, hopping those on
// another server into the zone's world first. The teacher must be in that
// world, since the teleport is executed by its bridge mod.
func (c *controller) teleportToZone(cc *proxy.ClientConn, inst *instanceData, z zoneData, names []string) (int, string) {
	x, y, zz, yaw, ok := z.teleportPoint()
	if !ok {
		return 0, "This zone has no teleport point."
	}
	if cc.ServerName() != inst.ProxyName {
		return 0, "Join this world first."
	}
	var targets []string
	hopped := false
	for _, name := range names {
		pcc := proxy.Find(name)
		if pcc == nil {
			continue
		}
		targets = append(targets, name)
		if pcc.ServerName() != inst.ProxyName {
			_ = c.hopPlayer(pcc, inst.ProxyName)
			hopped = true
		}
	}
	if len(targets) == 0 {
		return 0, "Nobody to teleport: nobody is online."
	}
	msg := map[string]interface{}{
		"action":  "tp_pos",
		"players": targets,
		"pos":     map[string]float64{"x": x, "y": y, "z": zz},
		"yaw":     yaw,
	}
	teacher := cc.Name()
	if hopped {
		go func() {
			time.Sleep(3 * time.Second)
			c.sendToPlayerServer(teacher, msg)
		}()
	} else {
		c.sendToPlayerServer(teacher, msg)
	}
	return len(targets), ""
}

// ── In-world zone editor sessions ───────────────────────────────────────────

func (c *controller) startZoneEdit(cc *proxy.ClientConn, inst *instanceData, name string, access zoneAccess, who string) {
	c.mu.Lock()
	c.runtime.zoneEdits[cc.Name()] = zoneEdit{InstanceID: inst.ID, Name: name, Access: access}
	c.mu.Unlock()
	if strings.TrimSpace(name) == "" {
		name = "New zone"
	}
	c.sendToPlayerServer(cc.Name(), map[string]interface{}{
		"action": "zone_edit_start",
		"player": cc.Name(),
		"name":   name,
		"who":    who,
	})
}

func (c *controller) takeZoneEdit(player string) (zoneEdit, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.runtime.zoneEdits[player]
	delete(c.runtime.zoneEdits, player)
	return e, ok
}

func vec3(m map[string]interface{}, key string) ([3]float64, bool) {
	p, _ := m[key].(map[string]interface{})
	x, okX := numberField(p, "x")
	y, okY := numberField(p, "y")
	z, okZ := numberField(p, "z")
	return [3]float64{x, y, z}, okX && okY && okZ
}

// handleZoneEditDone saves the zone drawn with the in-world editor.
func (c *controller) handleZoneEditDone(cc *proxy.ClientConn, data map[string]interface{}) {
	edit, ok := c.takeZoneEdit(cc.Name())
	if !ok {
		return
	}
	inst, err := c.getInstanceByID(edit.InstanceID)
	if err != nil || inst == nil || inst.ProxyName != cc.ServerName() || !c.canManageInstance(inst, cc.Name()) {
		c.notify(cc, "The zone could not be saved: you are no longer in that world.")
		return
	}
	p1, ok1 := vec3(data, "p1")
	p2, ok2 := vec3(data, "p2")
	tp, okTP := vec3(data, "tp")
	if !ok1 || !ok2 {
		c.notify(cc, "The zone could not be saved: corners are missing.")
		return
	}
	if !okTP {
		tp = p1
	}
	yaw, _ := numberField(data, "yaw")
	_, msg := c.createZone(inst, edit.Name, edit.Access, p1, p2, tp, yaw)
	c.notify(cc, msg)
	c.showWorldTools(cc)
}
