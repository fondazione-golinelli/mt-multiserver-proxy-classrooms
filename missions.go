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

// ── Zone missions ───────────────────────────────────────────────────────────
//
// One mission per zone (zone_missions). Goals and support tools reference
// catalog keys; the classrooms_bridge mod owns the catalog (it knows which
// items, animals and blocks exist in the world), counts progress and reports
// it back with "mission_progress".

const (
	maxMissionGoals = 6
	maxMissionTools = 8
)

var missionGoalTypes = [][2]string{
	{"deliver", "Deliver to the chest"},
	{"animals", "Animals in the zone"},
	{"blocks", "Blocks in the zone"},
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
	ZoneID      int
	Title       string
	Description string
	Goals       []missionGoal
	Tools       []missionTool
	CompletedAt sql.NullTime
}

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
	ZoneID      int
	Title       string
	Description string
	Goals       []missionGoal
	Tools       []missionTool
	Mode        string // "goals" or "tools"
	TypeIndex   int    // 1-based index in missionGoalTypes
	Target      string // selected catalog key
	Count       int    // amount for the next goal
	ToolAmount  int    // amount added per tool click
}

func newMissionDraft(zoneID int) *missionDraft {
	return &missionDraft{ZoneID: zoneID, Mode: "goals", TypeIndex: 1, Count: 10, ToolAmount: 1}
}

func (c *controller) getMission(zoneID int) (*missionData, error) {
	var m missionData
	var goals, tools string
	err := c.db.QueryRow(`SELECT id, zone_id, title, description, objectives, tools, completed_at
		FROM zone_missions WHERE zone_id = ?`, zoneID).Scan(
		&m.ID, &m.ZoneID, &m.Title, &m.Description, &goals, &tools, &m.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(goals), &m.Goals)
	_ = json.Unmarshal([]byte(tools), &m.Tools)
	return &m, nil
}

// getMissionsForWorld maps zone ID to its mission.
func (c *controller) getMissionsForWorld(instanceID string) (map[int]*missionData, error) {
	rows, err := c.db.Query(`SELECT m.id, m.zone_id, m.title, m.description, m.objectives, m.tools, m.completed_at
		FROM zone_missions m JOIN instance_zones z ON z.id = m.zone_id WHERE z.instance_id = ?`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]*missionData{}
	for rows.Next() {
		var m missionData
		var goals, tools string
		if err := rows.Scan(&m.ID, &m.ZoneID, &m.Title, &m.Description, &goals, &tools, &m.CompletedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(goals), &m.Goals)
		_ = json.Unmarshal([]byte(tools), &m.Tools)
		mm := m
		out[m.ZoneID] = &mm
	}
	return out, nil
}

func (c *controller) createMission(inst *instanceData, d *missionDraft) (bool, string) {
	title := cleanShortName(d.Title, 40)
	if title == "" {
		return false, "Give the mission a title."
	}
	if len(d.Goals) == 0 {
		return false, "Add at least one goal."
	}
	existing, err := c.getMission(d.ZoneID)
	if err != nil {
		return false, "Could not check the zone's mission."
	}
	if existing != nil {
		return false, "This zone already has a mission: delete it first."
	}
	goals, _ := json.Marshal(d.Goals)
	tools, _ := json.Marshal(d.Tools)
	if _, err := c.db.Exec(`INSERT INTO zone_missions (zone_id, title, description, objectives, tools)
		VALUES (?, ?, ?, ?, ?)`, d.ZoneID, title, cleanShortName(d.Description, 120), string(goals), string(tools)); err != nil {
		return false, "Could not save the mission."
	}
	c.pushZones(inst)
	return true, "Mission " + title + " started."
}

func (c *controller) deleteMission(inst *instanceData, zoneID int) {
	if m, _ := c.getMission(zoneID); m != nil {
		c.mu.Lock()
		delete(c.runtime.missionProgress, m.ID)
		c.mu.Unlock()
	}
	if _, err := c.db.Exec("DELETE FROM zone_missions WHERE zone_id = ?", zoneID); err != nil {
		log.Printf("[%s] delete mission of zone %d: %v", pluginName, zoneID, err)
	}
	c.pushZones(inst)
}

func (c *controller) getMissionProgress(missionID int) (missionProgress, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.runtime.missionProgress[missionID]
	return p, ok
}

// missionSummary is the short status shown on zone rows.
func (c *controller) missionSummary(m *missionData) (string, string) {
	if m.CompletedAt.Valid {
		return "Completed", success
	}
	p, ok := c.getMissionProgress(m.ID)
	if ok && p.Complete {
		return "Completed", success
	}
	done := 0
	for i, g := range m.Goals {
		if ok && i < len(p.Counts) && p.Counts[i] >= g.Count {
			done++
		}
	}
	return fmt.Sprintf("%d/%d goals", done, len(m.Goals)), warning
}

// missionPayload is the mission inside the set_zones zone entry.
func missionPayload(m *missionData, participants []string) map[string]interface{} {
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
	return map[string]interface{}{
		"id":           m.ID,
		"title":        m.Title,
		"description":  m.Description,
		"objectives":   goals,
		"tools":        tools,
		"participants": participants,
	}
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
		c.showMissionEditor(cc, draft.ZoneID)
	}
}

// openMissionEditor shows the editor, asking the bridge for the catalog
// first when it is not cached yet.
func (c *controller) openMissionEditor(cc *proxy.ClientConn, inst *instanceData, zoneID int) {
	c.mu.Lock()
	d := c.runtime.missionDrafts[cc.Name()]
	if d == nil || d.ZoneID != zoneID {
		c.runtime.missionDrafts[cc.Name()] = newMissionDraft(zoneID)
	}
	c.mu.Unlock()
	if _, ok := c.getCatalog(inst.ID); ok {
		c.showMissionEditor(cc, zoneID)
		return
	}
	c.sendToPlayerServer(cc.Name(), map[string]string{"action": "mission_catalog_request", "player": cc.Name()})
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
	switch g.Type {
	case "deliver":
		return fmt.Sprintf("Deliver %d %s", g.Count, g.Label)
	case "animals":
		return fmt.Sprintf("%d × %s in the zone", g.Count, g.Label)
	case "blocks":
		return fmt.Sprintf("%d × %s blocks in the zone", g.Count, g.Label)
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
