package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Missions ────────────────────────────────────────────────────────────────
//
// Zone missions (one per zone) and world missions (not tied to a zone,
// played by the class or one group) share the zone_missions table, so mission
// IDs are unique in the bridge's storage keys. Goals and support tools
// reference catalog keys; the classrooms_bridge mod owns the catalog (it
// knows which items, animals and blocks exist in the world), counts progress
// and reports it back with "mission_progress".

const (
	maxMissionGoals  = 6
	maxMissionTools  = 8
	maxWorldMissions = 8 // active world missions per world
)

var missionGoalTypes = [][2]string{
	{"deliver", "Deliver to the chest"},
	{"collect", "Gather in inventories"},
	{"animals", "Animals in the area"},
	{"blocks", "Blocks in the area"},
}

type missionGoal struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Count int    `json:"count"`
}

type missionTool struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Count int    `json:"count"`
}

// addGoal adds a goal, summing the amount into an existing goal of the same
// type and target instead of listing it twice.
func addGoal(goals []missionGoal, g missionGoal, maxCount int) ([]missionGoal, bool) {
	for i := range goals {
		if goals[i].Type == g.Type && goals[i].Key == g.Key {
			goals[i].Count = min(goals[i].Count+g.Count, maxCount)
			return goals, true
		}
	}
	if len(goals) >= maxMissionGoals {
		return goals, false
	}
	return append(goals, g), true
}

// addTool adds support tools, summing amounts of the same item.
func addTool(tools []missionTool, t missionTool, maxCount int) ([]missionTool, bool) {
	for i := range tools {
		if tools[i].Key == t.Key {
			tools[i].Count = min(tools[i].Count+t.Count, maxCount)
			return tools, true
		}
	}
	if len(tools) >= maxMissionTools {
		return tools, false
	}
	return append(tools, t), true
}

type missionData struct {
	ID          int
	ZoneID      int // 0 for a world mission
	InstanceID  string
	GroupID     int // world missions: 0 = the whole class
	Title       string
	Description string
	Goals       []missionGoal
	Tools       []missionTool
	CompletedAt sql.NullTime
}

func (m *missionData) global() bool { return m.ZoneID == 0 }

type catalogEntry struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"` // item name shown as the entry's picture
}

// missionCatalog is what the bridge reports for one world.
type missionCatalog map[string][]catalogEntry

type missionProgress struct {
	Counts   []int
	Complete bool
	Chest    bool
	Updated  time.Time
}

// missionDraft is the mission a teacher is composing in the editor.
type missionDraft struct {
	ZoneID      int // 0 for a world mission
	GroupID     int // world missions: 0 = the whole class
	Title       string
	Description string
	Goals       []missionGoal
	Tools       []missionTool
	Mode        string // "goals" or "tools"
	TypeIndex   int    // 1-based index in missionGoalTypes
	Target      string // selected catalog key
	Count       int    // amount for the next goal
	ToolAmount  int    // amount added per tool click
	EditingID   int    // mission being edited; 0 when creating a new one
}

func (d *missionDraft) global() bool { return d.ZoneID == 0 }

func newMissionDraft(zoneID int) *missionDraft {
	return &missionDraft{ZoneID: zoneID, Mode: "goals", TypeIndex: 1, Count: 10, ToolAmount: 1}
}

func newWorldMissionDraft(groupID int) *missionDraft {
	d := newMissionDraft(0)
	d.GroupID = groupID
	return d
}

// draftFromMission loads a mission into the editor to change it.
func draftFromMission(m *missionData) *missionDraft {
	d := newMissionDraft(m.ZoneID)
	d.GroupID, d.EditingID, d.Title, d.Description = m.GroupID, m.ID, m.Title, m.Description
	d.Goals = append([]missionGoal(nil), m.Goals...)
	d.Tools = append([]missionTool(nil), m.Tools...)
	return d
}

// needsChest tells whether goals need the delivery chest: deliveries, and for
// world missions also animals and blocks, counted around the chest.
func needsChest(goals []missionGoal, global bool) bool {
	for _, g := range goals {
		if g.Type == "deliver" || (global && (g.Type == "animals" || g.Type == "blocks")) {
			return true
		}
	}
	return false
}

const missionSelect = `SELECT m.id, COALESCE(m.zone_id, 0), COALESCE(m.instance_id, z.instance_id, ''),
	COALESCE(m.group_id, 0), m.title, m.description, m.objectives, m.tools, m.completed_at
	FROM zone_missions m LEFT JOIN instance_zones z ON z.id = m.zone_id `

func (c *controller) queryMissions(where string, args ...any) ([]missionData, error) {
	rows, err := c.db.Query(missionSelect+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []missionData
	for rows.Next() {
		var m missionData
		var goals, tools string
		if err := rows.Scan(&m.ID, &m.ZoneID, &m.InstanceID, &m.GroupID, &m.Title, &m.Description,
			&goals, &tools, &m.CompletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(goals), &m.Goals)
		_ = json.Unmarshal([]byte(tools), &m.Tools)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (c *controller) queryMission(where string, args ...any) (*missionData, error) {
	list, err := c.queryMissions(where, args...)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// getMission returns the mission of a zone.
func (c *controller) getMission(zoneID int) (*missionData, error) {
	return c.queryMission("WHERE m.zone_id = ?", zoneID)
}

func (c *controller) getMissionByID(id int) (*missionData, error) {
	return c.queryMission("WHERE m.id = ?", id)
}

// getWorldMissionList lists every mission of a world: active first.
func (c *controller) getWorldMissionList(instanceID string) ([]missionData, error) {
	return c.queryMissions(`WHERE COALESCE(m.instance_id, z.instance_id) = ?
		ORDER BY m.completed_at IS NOT NULL, m.id`, instanceID)
}

// getMissionsForWorld maps zone ID to its mission.
func (c *controller) getMissionsForWorld(instanceID string) (map[int]*missionData, error) {
	list, err := c.queryMissions("WHERE z.instance_id = ?", instanceID)
	if err != nil {
		return nil, err
	}
	out := map[int]*missionData{}
	for i := range list {
		out[list[i].ZoneID] = &list[i]
	}
	return out, nil
}

func nullableID(id int) any {
	if id == 0 {
		return nil
	}
	return id
}

func (c *controller) createMission(inst *instanceData, d *missionDraft) (bool, string, int) {
	title := cleanShortName(d.Title, 40)
	if title == "" {
		return false, "Give the mission a title.", 0
	}
	if len(d.Goals) == 0 {
		return false, "Add at least one goal.", 0
	}
	if d.global() {
		list, err := c.getWorldMissionList(inst.ID)
		if err != nil {
			return false, "Could not check the world's missions.", 0
		}
		active := 0
		for _, m := range list {
			if m.global() && !m.CompletedAt.Valid {
				active++
			}
		}
		if active >= maxWorldMissions {
			return false, fmt.Sprintf("At most %d world missions in progress: delete one first.", maxWorldMissions), 0
		}
	} else {
		existing, err := c.getMission(d.ZoneID)
		if err != nil {
			return false, "Could not check the zone's mission.", 0
		}
		if existing != nil {
			return false, "This zone already has a mission: delete it first.", 0
		}
	}
	goals, _ := json.Marshal(d.Goals)
	tools, _ := json.Marshal(d.Tools)
	res, err := c.db.Exec(`INSERT INTO zone_missions (zone_id, instance_id, group_id, title, description, objectives, tools)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, nullableID(d.ZoneID), inst.ID, nullableID(d.GroupID), title,
		cleanShortName(d.Description, 120), string(goals), string(tools))
	if err != nil {
		return false, "Could not save the mission.", 0
	}
	id, _ := res.LastInsertId()
	c.pushZones(inst)
	return true, "Mission " + title + " started.", int(id)
}

// updateMission saves edited goals and tools of a mission still in progress.
// The mission keeps its ID, so tools already handed out and progress stay.
func (c *controller) updateMission(inst *instanceData, d *missionDraft) (bool, string) {
	title := cleanShortName(d.Title, 40)
	if title == "" {
		return false, "Give the mission a title."
	}
	if len(d.Goals) == 0 {
		return false, "Add at least one goal."
	}
	if m, err := c.getMissionByID(d.EditingID); err != nil || m == nil || m.InstanceID != inst.ID {
		return false, "The mission no longer exists."
	}
	goals, _ := json.Marshal(d.Goals)
	tools, _ := json.Marshal(d.Tools)
	res, err := c.db.Exec(`UPDATE zone_missions SET title = ?, description = ?, objectives = ?, tools = ?
		WHERE id = ? AND completed_at IS NULL`,
		title, cleanShortName(d.Description, 120), string(goals), string(tools), d.EditingID)
	if err != nil {
		return false, "Could not save the mission."
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, "The mission is already completed: it can no longer be changed."
	}
	// Goal indexes may have changed: wait for a fresh report.
	c.mu.Lock()
	delete(c.runtime.missionProgress, d.EditingID)
	c.mu.Unlock()
	c.pushZones(inst)
	return true, "Mission " + title + " updated."
}

func (c *controller) deleteMission(inst *instanceData, missionID int) {
	c.mu.Lock()
	delete(c.runtime.missionProgress, missionID)
	c.mu.Unlock()
	if _, err := c.db.Exec("DELETE FROM zone_missions WHERE id = ?", missionID); err != nil {
		log.Printf("[%s] delete mission %d: %v", pluginName, missionID, err)
	}
	c.pushZones(inst)
}

func (c *controller) getMissionProgress(missionID int) (missionProgress, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.runtime.missionProgress[missionID]
	return p, ok
}

// missionComplete tells whether a mission is done, from the database or the
// latest report.
func (c *controller) missionComplete(m *missionData) bool {
	if m.CompletedAt.Valid {
		return true
	}
	p, ok := c.getMissionProgress(m.ID)
	return ok && p.Complete
}

// missionSummary is the short status shown on zone and mission rows.
func (c *controller) missionSummary(m *missionData) (string, string) {
	if c.missionComplete(m) {
		return "Completed", success
	}
	p, ok := c.getMissionProgress(m.ID)
	done := 0
	for i, g := range m.Goals {
		if ok && i < len(p.Counts) && p.Counts[i] >= g.Count {
			done++
		}
	}
	return fmt.Sprintf("%d/%d goals", done, len(m.Goals)), warning
}

// missionPayload is a mission as sent to the bridge: inside its zone entry
// of set_zones, or in the "missions" list for world missions.
// worldMissionColor is the color of world missions played by the class.
const worldMissionColor = "#4fb3ff"

func missionPayload(m *missionData, participants []string, color string) map[string]interface{} {
	goals := make([]map[string]interface{}, 0, len(m.Goals))
	for _, g := range m.Goals {
		goals = append(goals, map[string]interface{}{"type": g.Type, "key": g.Key, "label": g.Label, "count": g.Count})
	}
	tools := make([]map[string]interface{}, 0, len(m.Tools))
	for _, t := range m.Tools {
		tools = append(tools, map[string]interface{}{"key": t.Key, "label": t.Label, "count": t.Count})
	}
	if participants == nil {
		participants = []string{}
	}
	out := map[string]interface{}{
		"id":           m.ID,
		"title":        m.Title,
		"description":  m.Description,
		"objectives":   goals,
		"tools":        tools,
		"participants": participants,
		"color":        color,
	}
	if m.global() {
		out["global"] = true
	}
	return out
}

// handleMissionProgress stores a progress report from the bridge.
func (c *controller) handleMissionProgress(data map[string]interface{}) {
	id, ok := numberField(data, "mission")
	if !ok {
		return
	}
	p := missionProgress{Updated: time.Now()}
	if counts, ok := data["counts"].([]interface{}); ok {
		for _, v := range counts {
			n, _ := v.(float64)
			p.Counts = append(p.Counts, int(n))
		}
	}
	p.Complete, _ = data["complete"].(bool)
	p.Chest, _ = data["chest"].(bool)
	c.mu.Lock()
	c.runtime.missionProgress[int(id)] = p
	c.mu.Unlock()
	if p.Complete {
		if _, err := c.db.Exec("UPDATE zone_missions SET completed_at = CURRENT_TIMESTAMP WHERE id = ? AND completed_at IS NULL",
			int(id)); err != nil {
			log.Printf("[%s] mark mission %d complete: %v", pluginName, int(id), err)
		}
	}
}

// ── Catalog ─────────────────────────────────────────────────────────────────

func (c *controller) getCatalog(instanceID string) (missionCatalog, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cat, ok := c.runtime.missionCatalogs[instanceID]
	return cat, ok
}

// handleMissionCatalog caches the catalog sent by the bridge and opens the
// editor the teacher was waiting for.
func (c *controller) handleMissionCatalog(cc *proxy.ClientConn, data map[string]interface{}) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	raw, _ := json.Marshal(data["catalog"])
	var cat missionCatalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return
	}
	c.mu.Lock()
	c.runtime.missionCatalogs[inst.ID] = cat
	draft := c.runtime.missionDrafts[cc.Name()]
	c.mu.Unlock()
	if draft != nil {
		c.showMissionEditor(cc)
	}
}

// openMission shows an existing mission (missionID > 0) or the editor for a
// new one described by draft, asking the bridge for the catalog first when it
// is not cached yet.
func (c *controller) openMission(cc *proxy.ClientConn, inst *instanceData, missionID int, draft *missionDraft) {
	c.mu.Lock()
	c.runtime.missionView[cc.Name()] = missionID
	if draft != nil {
		d := c.runtime.missionDrafts[cc.Name()]
		// Keep a draft in progress for the same target.
		if d == nil || d.EditingID != 0 || d.ZoneID != draft.ZoneID || d.GroupID != draft.GroupID {
			c.runtime.missionDrafts[cc.Name()] = draft
		}
	}
	c.mu.Unlock()
	if _, ok := c.getCatalog(inst.ID); ok || missionID != 0 {
		c.showMissionEditor(cc)
		return
	}
	c.sendToPlayerServer(cc.Name(), map[string]string{"action": "mission_catalog_request", "player": cc.Name()})
}

// openZoneMission opens a zone's mission, or a new one for it.
func (c *controller) openZoneMission(cc *proxy.ClientConn, inst *instanceData, zoneID int) {
	if m, err := c.getMission(zoneID); err == nil && m != nil {
		c.openMission(cc, inst, m.ID, nil)
		return
	}
	c.openMission(cc, inst, 0, newMissionDraft(zoneID))
}

func (c *controller) missionDraftFor(player string) *missionDraft {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.missionDrafts[player]
}

func goalTypeLabel(t string) string {
	for _, gt := range missionGoalTypes {
		if gt[0] == t {
			return gt[1]
		}
	}
	return t
}

func goalText(g missionGoal) string {
	return goalTextFor(g, false)
}

// goalTextFor describes a goal; world missions count animals and blocks
// around their delivery chest.
func goalTextFor(g missionGoal, global bool) string {
	where := "in the zone"
	if global {
		where = "near the chest"
	}
	switch g.Type {
	case "deliver":
		return fmt.Sprintf("Deliver %d %s", g.Count, g.Label)
	case "collect":
		return fmt.Sprintf("Gather %d %s", g.Count, g.Label)
	case "animals":
		return fmt.Sprintf("%d × %s %s", g.Count, g.Label, where)
	case "blocks":
		return fmt.Sprintf("%d × %s blocks %s", g.Count, g.Label, where)
	}
	return g.Label
}

func toolsText(tools []missionTool) string {
	if len(tools) == 0 {
		return "none"
	}
	parts := make([]string, len(tools))
	for i, t := range tools {
		parts[i] = fmt.Sprintf("%s ×%d", t.Label, t.Count)
	}
	return strings.Join(parts, ", ")
}
