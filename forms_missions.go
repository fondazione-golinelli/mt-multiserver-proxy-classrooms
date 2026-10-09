package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── World Tools → Mission editor ────────────────────────────────────────────
//
// Two steps (Goals, Tools for students) on the left, the mission being built
// on the right. Choices are pictures of the real items, animals (spawn eggs)
// and blocks, so the teacher sees exactly what they assign.

const (
	maxGoalCount = 999
	maxToolCount = 99
)

var goalTypeHints = map[string]string{
	"deliver": "Students put these items into the delivery chest.",
	"collect": "Students hold these items in their inventories (all together).",
	"animals": "These animals must be inside the zone at the same time.",
	"blocks":  "These blocks must be placed inside the zone.",
}

// World missions have no zone: animals and blocks count around the chest.
var worldGoalTypeHints = map[string]string{
	"animals": "These animals must be near the delivery chest (16 blocks).",
	"blocks":  "These blocks must be placed near the delivery chest (16 blocks).",
}

func goalTypeHint(goalType string, global bool) string {
	if h, ok := worldGoalTypeHints[goalType]; ok && global {
		return h
	}
	return goalTypeHints[goalType]
}

// catalogKind is the catalog list a goal type picks from.
func catalogKind(goalType string) string {
	if goalType == "collect" {
		return "deliver"
	}
	return goalType
}

var toolAmounts = []int{1, 4, 16}

// Short labels for the goal type buttons; goalTypeHints explains each.
var goalTypeButtons = []string{"Deliver", "Gather", "Animals", "Blocks"}

func clampIndex(i, n int) int {
	if i < 1 || i > n {
		return 1
	}
	return i
}

// itemIcon is the client-side name of a world item in proxy formspecs: the
// proxy prefixes item names with the world's media pool.
func (c *controller) itemIcon(name string) string {
	if name == "" {
		return ""
	}
	pool := c.cfg.Instance.MediaPool
	if pool == "" {
		return name
	}
	return pool + "_" + name
}

// pictureButton is an item picture button, or a text button when the entry
// has no item to show.
func (c *controller) pictureButton(b *strings.Builder, x, y, size float64, name, icon, label string, selected bool) {
	color := colorButton
	if selected {
		color = colorTabFocus
	}
	b.WriteString(fmt.Sprintf("style[%s;bgcolor=%s]", name, color))
	if icon != "" {
		b.WriteString(fmt.Sprintf("item_image_button[%g,%g;%g,%g;%s;%s;]", x, y, size, size, fmtEsc(c.itemIcon(icon)), name))
	} else {
		short := []rune(label)
		if len(short) > 6 {
			short = short[:6]
		}
		b.WriteString(fmt.Sprintf("button[%g,%g;%g,%g;%s;%s]", x, y, size, size, name, fmtEsc(string(short))))
	}
	b.WriteString(tooltip(name, label))
}

func (c *controller) smallIcon(b *strings.Builder, x, y, size float64, icon string) {
	if icon != "" {
		b.WriteString(fmt.Sprintf("item_image[%g,%g;%g,%g;%s]", x, y, size, size, fmtEsc(c.itemIcon(icon))))
	}
}

// missionAudience describes who plays a zone's mission.
func (c *controller) missionAudience(inst *instanceData, z zoneData) string {
	if z.GroupID.Valid && !z.Open && inst.ClassID != nil {
		if g, _ := c.getGroup(*inst.ClassID, int(z.GroupID.Int64)); g != nil {
			return "Group " + g.Name
		}
	}
	return "The whole class"
}

// worldMissionAudience describes who plays a world mission.
func (c *controller) worldMissionAudience(inst *instanceData, groupID int) string {
	if groupID != 0 && inst.ClassID != nil {
		if g, _ := c.getGroup(*inst.ClassID, groupID); g != nil {
			return "Group " + g.Name
		}
	}
	return "The whole class"
}

// missionWhere returns where a mission is played and by whom; ok is false
// when its zone no longer exists.
func (c *controller) missionWhere(inst *instanceData, zoneID, groupID int) (place, audience string, ok bool) {
	if zoneID == 0 {
		return "Whole world", c.worldMissionAudience(inst, groupID), true
	}
	z := c.zoneByID(inst, zoneID)
	if z == nil {
		return "", "", false
	}
	return z.Name, c.missionAudience(inst, *z), true
}

func (c *controller) zoneByID(inst *instanceData, zoneID int) *zoneData {
	zones, _ := c.getZones(inst.ID)
	for i := range zones {
		if zones[i].ID == zoneID {
			return &zones[i]
		}
	}
	return nil
}

func progressBar(b *strings.Builder, x, y, w float64, have, need int, color string) {
	b.WriteString(box(x, y, w, 0.3, "#151b2e"))
	ratio := float64(have) / float64(max(need, 1))
	if ratio > 1 {
		ratio = 1
	}
	if ratio > 0 {
		b.WriteString(box(x, y, w*ratio, 0.3, color))
	}
}

func goalSentence(goalType string, count int, label string, global bool) string {
	where := "inside the zone"
	if global {
		where = "near the delivery chest"
	}
	switch goalType {
	case "deliver":
		return fmt.Sprintf("Deliver %d × %s into the delivery chest", count, label)
	case "collect":
		return fmt.Sprintf("Gather %d × %s in the students' inventories", count, label)
	case "animals":
		return fmt.Sprintf("Have %d × %s %s", count, label, where)
	case "blocks":
		return fmt.Sprintf("Place %d × %s blocks %s", count, label, where)
	}
	return label
}

func findEntry(entries []catalogEntry, key string) *catalogEntry {
	for i := range entries {
		if entries[i].Key == key {
			return &entries[i]
		}
	}
	return nil
}

// showMissionEditor shows the mission in missionView: its status, or the
// editor while changing it; with no mission, the editor for the draft.
func (c *controller) showMissionEditor(cc *proxy.ClientConn) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	c.mu.RLock()
	view := c.runtime.missionView[cc.Name()]
	draft := c.runtime.missionDrafts[cc.Name()]
	c.mu.RUnlock()

	var mission *missionData
	if view != 0 {
		m, err := c.getMissionByID(view)
		if err != nil {
			c.notify(cc, "Could not load the mission: "+err.Error())
			return
		}
		if m == nil || m.InstanceID != inst.ID {
			c.notify(cc, "The mission no longer exists.")
			c.showWorldTools(cc)
			return
		}
		mission = m
	}
	editing := mission != nil && draft != nil && draft.EditingID == mission.ID && !c.missionComplete(mission)
	var zoneID, groupID int
	switch {
	case mission != nil:
		zoneID, groupID = mission.ZoneID, mission.GroupID
	case draft != nil:
		zoneID, groupID = draft.ZoneID, draft.GroupID
	default:
		c.showWorldTools(cc)
		return
	}
	place, audience, ok := c.missionWhere(inst, zoneID, groupID)
	if !ok {
		c.showWorldTools(cc)
		return
	}
	if mission != nil && !editing {
		var b strings.Builder
		fsOpen(&b, 12, 10.4)
		fsHeader(&b, 12, "Mission · "+place, "Played by: "+audience, true, true)
		c.writeMissionStatus(&b, cc, mission)
		cc.ShowFormspec("classrooms:mission", b.String())
		return
	}
	if mission == nil && draft.EditingID != 0 {
		draft = newMissionDraft(zoneID)
		draft.GroupID = groupID
		c.mu.Lock()
		c.runtime.missionDrafts[cc.Name()] = draft
		c.mu.Unlock()
	}
	global := draft.global()
	catalog, _ := c.getCatalog(inst.ID)

	const w, h = 14.2, 11.2
	var b strings.Builder
	fsOpen(&b, w, h)
	heading := "New mission · "
	if editing {
		heading = "Edit mission · "
	}
	fsHeader(&b, w, heading+place, "Played by: "+audience, true, true)

	// Title and description.
	b.WriteString(box(0.3, 1.3, 13.6, 1.45, colorCard))
	b.WriteString(fmt.Sprintf("field[0.55,1.9;5.4,0.6;ms_title;Mission title;%s]", fmtEsc(draft.Title)))
	b.WriteString(fmt.Sprintf("field[6.15,1.9;7.5,0.6;ms_desc;Short explanation for students (optional);%s]", fmtEsc(draft.Description)))
	b.WriteString("field_close_on_enter[ms_title;false]field_close_on_enter[ms_desc;false]")

	// Left: builder.
	b.WriteString(box(0.3, 2.95, 8.75, 7.05, colorCard))
	tabBar(&b, 0.5, 3.1, 4.1, "ms_mode_"+draft.Mode, [][2]string{
		{"ms_mode_goals", "1 · Goals"},
		{"ms_mode_tools", "2 · Tools for students"},
	})
	if draft.Mode == "tools" {
		c.writeToolPicker(&b, draft, catalog["tools"], global)
	} else {
		c.writeGoalBuilder(&b, draft, catalog, global)
	}

	// Right: the mission so far.
	b.WriteString(box(9.25, 2.95, 4.65, 7.05, colorCard))
	b.WriteString(sectionTitle(9.45, 3.25, fmt.Sprintf("This mission · goals %d/%d", len(draft.Goals), maxMissionGoals)))
	if len(draft.Goals) == 0 {
		b.WriteString(hint(9.5, 3.7, "No goals yet."))
	}
	gy := 3.55
	for i, g := range draft.Goals {
		b.WriteString(box(9.4, gy, 4.35, 0.62, colorRow))
		c.smallIcon(&b, 9.48, gy+0.06, 0.5, g.Icon)
		text := []rune(goalTextFor(g, global))
		if len(text) > 23 {
			text = append(text[:22], '…')
		}
		b.WriteString(fmt.Sprintf("label[10.08,%g;%s]", gy+0.31, fmtEsc(string(text))))
		b.WriteString(iconBtn(13.33, gy+0.13, 0.36, fmt.Sprintf("ms_rm_%d", i), iconClose, "Remove: "+goalTextFor(g, global)))
		gy += 0.68
	}
	b.WriteString(sectionTitle(9.45, 7.75, "Tools for each student"))
	if len(draft.Tools) == 0 {
		b.WriteString(hint(9.5, 8.15, "None (optional)."))
	}
	for i, t := range draft.Tools {
		col, row := i%2, i/2
		x, y := 9.4+float64(col)*2.2, 7.95+float64(row)*0.5
		b.WriteString(box(x, y, 2.12, 0.44, colorRow))
		c.smallIcon(&b, x+0.04, y+0.02, 0.4, t.Icon)
		b.WriteString(fmt.Sprintf("label[%g,%g;%s]", x+0.5, y+0.22, fmtEsc(fmt.Sprintf("×%d", t.Count))))
		b.WriteString(iconBtn(x+1.72, y+0.06, 0.32, fmt.Sprintf("ms_tool_rm_%d", i), iconClose, "Remove "+t.Label))
	}

	// Footer.
	if editing {
		b.WriteString(hint(0.35, 10.6, "Progress and tools already given are kept."))
	} else if needsChest(draft.Goals, global) {
		b.WriteString(fmt.Sprintf("image[0.35,10.38;0.42,0.42;%s]", iconWarning))
		where := "inside the zone"
		if global {
			where = "anywhere"
		}
		b.WriteString(hint(0.9, 10.6, "After starting you get the delivery chest: place it "+where+"."))
	}
	if editing {
		b.WriteString(btn(8.25, 10.2, 2.4, 0.8, "ms_cancel_edit", "Cancel"))
		b.WriteString(styledBtn(10.8, 10.2, 3.1, 0.8, "ms_start", "Save changes", colorPrimary))
	} else {
		b.WriteString(styledBtn(10.8, 10.2, 3.1, 0.8, "ms_start", "Start mission", colorPrimary))
	}

	cc.ShowFormspec("classrooms:mission", b.String())
}

func (c *controller) writeGoalBuilder(b *strings.Builder, draft *missionDraft, catalog missionCatalog, global bool) {
	b.WriteString(sectionTitle(0.55, 4.05, "What must students do?"))
	typeIndex := clampIndex(draft.TypeIndex, len(missionGoalTypes))
	for i := range missionGoalTypes {
		color := colorButton
		if i+1 == typeIndex {
			color = colorTabFocus
		}
		b.WriteString(styledBtn(0.5+float64(i)*2.07, 4.3, 2.0, 0.7, fmt.Sprintf("ms_type_%d", i+1), goalTypeButtons[i], color))
		b.WriteString(tooltip(fmt.Sprintf("ms_type_%d", i+1), missionGoalTypes[i][1]))
	}
	goalType := missionGoalTypes[typeIndex-1][0]
	b.WriteString(hint(0.55, 5.25, goalTypeHint(goalType, global)))

	entries := catalog[catalogKind(goalType)]
	if len(entries) == 0 {
		b.WriteString(hint(0.55, 5.8, "Nothing of this kind exists in this world."))
		return
	}
	const perRow, cell = 8, 1.03
	rows := (len(entries) + perRow - 1) / perRow
	b.WriteString(scrollbarFor("scr_ms_pick", 8.75, 5.5, 2.15, rows, cell, 0.05))
	b.WriteString("scroll_container[0.5,5.5;8.2,2.15;scr_ms_pick;vertical;0.1]")
	for i, e := range entries {
		x := float64(i%perRow) * cell
		y := 0.05 + float64(i/perRow)*cell
		c.pictureButton(b, x, y, 0.95, "ms_pick_"+e.Key, e.Icon, e.Label, e.Key == draft.Target)
	}
	b.WriteString("scroll_container_end[]")

	// Amount and preview.
	b.WriteString(sectionTitle(0.55, 7.95, "How many?"))
	b.WriteString(btn(2.6, 7.7, 0.85, 0.55, "ms_amt_m10", "−10"))
	b.WriteString(btn(3.5, 7.7, 0.7, 0.55, "ms_amt_m1", "−1"))
	b.WriteString(fmt.Sprintf("field[4.25,7.7;1.0,0.55;ms_count;;%d]", draft.Count))
	b.WriteString("field_close_on_enter[ms_count;false]")
	b.WriteString(btn(5.3, 7.7, 0.7, 0.55, "ms_amt_p1", "+1"))
	b.WriteString(btn(6.05, 7.7, 0.85, 0.55, "ms_amt_p10", "+10"))

	selected := findEntry(entries, draft.Target)
	b.WriteString(box(0.5, 8.45, 8.35, 1.4, colorRow))
	if selected == nil {
		b.WriteString(hint(0.7, 8.85, "Pick a picture above."))
	} else {
		c.smallIcon(b, 0.62, 8.62, 0.6, selected.Icon)
		b.WriteString(coloredLbl(1.35, 8.92, warning, goalSentence(goalType, draft.Count, selected.Label, global)))
	}
	b.WriteString(styledBtn(6.15, 9.25, 2.6, 0.5, "ms_add", "+ Add this goal", colorPrimary))
}

func (c *controller) writeToolPicker(b *strings.Builder, draft *missionDraft, tools []catalogEntry, global bool) {
	if global {
		b.WriteString(sectionTitle(0.55, 4.05, "Given once to each student in this world"))
	} else {
		b.WriteString(sectionTitle(0.55, 4.05, "Given once to each student entering the zone"))
	}
	b.WriteString(hint(0.55, 4.45, "Choose how many per click, then click the items."))
	b.WriteString(sectionTitle(0.55, 5.0, "Per click"))
	for i, n := range toolAmounts {
		color := colorButton
		if n == draft.ToolAmount {
			color = colorTabFocus
		}
		b.WriteString(styledBtn(2.2+float64(i)*1.05, 4.75, 0.95, 0.5, fmt.Sprintf("ms_toolamt_%d", n), fmt.Sprintf("+%d", n), color))
	}
	if len(tools) == 0 {
		b.WriteString(hint(0.55, 5.4, "No tools available in this world."))
		return
	}
	const perRow, cell = 8, 1.03
	rows := (len(tools) + perRow - 1) / perRow
	b.WriteString(scrollbarFor("scr_ms_tools", 8.75, 5.45, 4.4, rows, cell, 0.05))
	b.WriteString("scroll_container[0.5,5.45;8.2,4.4;scr_ms_tools;vertical;0.1]")
	for i, t := range tools {
		x := float64(i%perRow) * cell
		y := 0.05 + float64(i/perRow)*cell
		c.pictureButton(b, x, y, 0.95, "ms_tool_"+t.Key, t.Icon, fmt.Sprintf("Add %d × %s", draft.ToolAmount, t.Label), false)
	}
	b.WriteString("scroll_container_end[]")
}

func (c *controller) writeMissionStatus(b *strings.Builder, cc *proxy.ClientConn, m *missionData) {
	p, hasProgress := c.getMissionProgress(m.ID)
	complete := c.missionComplete(m)
	global := m.global()

	b.WriteString(box(0.3, 1.3, 11.4, 1.6, colorCard))
	b.WriteString(coloredLbl(0.55, 1.65, light, m.Title))
	if m.Description != "" {
		b.WriteString(hint(0.55, 2.05, m.Description))
	}
	switch {
	case complete:
		b.WriteString(fmt.Sprintf("image[0.55,2.3;0.42,0.42;%s]", iconCheck))
		label := "Completed"
		if m.CompletedAt.Valid {
			label += " on " + m.CompletedAt.Time.Local().Format("02/01 15:04")
		}
		b.WriteString(coloredLbl(1.1, 2.5, success, label))
	case !hasProgress:
		b.WriteString(hint(0.55, 2.5, "Waiting for progress: it updates while someone is in this world."))
	default:
		summary, color := c.missionSummary(m)
		b.WriteString(coloredLbl(0.55, 2.5, color, "In progress · "+summary))
	}

	b.WriteString(box(0.3, 3.1, 11.4, 4.0, colorCard))
	b.WriteString(sectionTitle(0.55, 3.4, "Goals"))
	gy := 3.7
	for i, g := range m.Goals {
		have := 0
		if hasProgress && i < len(p.Counts) {
			have = p.Counts[i]
		}
		color := colorPrimary
		if complete || have >= g.Count {
			color = "#3fb56b"
			have = max(have, g.Count)
		}
		c.smallIcon(b, 0.55, gy-0.05, 0.45, g.Icon)
		b.WriteString(fmt.Sprintf("label[1.15,%g;%s]", gy+0.17, fmtEsc(goalTextFor(g, global))))
		progressBar(b, 6.4, gy+0.02, 3.9, min(have, g.Count), g.Count, color)
		b.WriteString(fmt.Sprintf("label[10.45,%g;%s]", gy+0.17, fmtEsc(fmt.Sprintf("%d/%d", min(have, g.Count), g.Count))))
		gy += 0.55
	}

	b.WriteString(box(0.3, 7.3, 11.4, 1.75, colorCard))
	toolsTitle, chestHelp, retoolsHelp := "Tools given in the zone",
		"Place it inside this zone: students put the requested items in it",
		"Every student gets the support tools again the next time they are in the zone"
	if global {
		toolsTitle, chestHelp, retoolsHelp = "Tools given to each student",
			"Place it anywhere: students bring the items there; animals and blocks count within 16 blocks of it",
			"Every student gets the support tools again the next time they are in this world"
	}
	b.WriteString(sectionTitle(0.55, 7.6, toolsTitle))
	if len(m.Tools) == 0 {
		b.WriteString(hint(0.55, 8.0, "None."))
	}
	for i, t := range m.Tools {
		x := 0.5 + float64(i)*1.4
		b.WriteString(box(x, 7.85, 1.32, 0.5, colorRow))
		c.smallIcon(b, x+0.05, 7.88, 0.44, t.Icon)
		b.WriteString(fmt.Sprintf("label[%g,8.1;%s]", x+0.58, fmtEsc(fmt.Sprintf("×%d", t.Count))))
		b.WriteString(fmt.Sprintf("tooltip[%g,7.85;1.32,0.5;%s]", x, fmtEsc(fmt.Sprintf("%s ×%d", t.Label, t.Count))))
	}
	if needsChest(m.Goals, global) {
		if hasProgress && p.Chest {
			b.WriteString(hint(0.55, 8.6, "Delivery chest: placed."))
		} else {
			b.WriteString(coloredLbl(0.55, 8.6, warning, "Delivery chest: not placed yet."))
		}
		b.WriteString(btn(7.0, 8.35, 4.5, 0.55, "ms_chest", "Give me the delivery chest"))
		b.WriteString(tooltip("ms_chest", chestHelp))
	}

	if c.isDeleteArmed(cc.Name(), fmt.Sprintf("mission:%d", m.ID)) {
		b.WriteString(coloredLbl(0.55, 9.65, danger, "Sure? Progress is lost."))
		b.WriteString(styledBtn(8.0, 9.3, 3.7, 0.75, "ms_delete", "Yes, delete mission", colorDanger))
		return
	}
	if !complete {
		b.WriteString(btn(0.3, 9.3, 3.6, 0.75, "ms_edit", "Edit goals & tools"))
		b.WriteString(tooltip("ms_edit", "Change goals and support tools: progress and tools already given stay"))
		b.WriteString(btn(4.05, 9.3, 3.75, 0.75, "ms_retools", "Give tools again"))
		b.WriteString(tooltip("ms_retools", retoolsHelp))
	} else {
		b.WriteString(hint(0.55, 9.65, "Delete it to create a new one."))
	}
	b.WriteString(styledBtn(8.0, 9.3, 3.7, 0.75, "ms_delete", "Delete mission", colorDanger))
}

func (c *controller) handleMissionEditor(cc *proxy.ClientConn, fields []mt.Field) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	c.mu.RLock()
	view := c.runtime.missionView[cc.Name()]
	draft := c.runtime.missionDrafts[cc.Name()]
	c.mu.RUnlock()
	fm := fieldMap(fields)
	has := func(k string) bool { _, ok := fm[k]; return ok }
	if !has("ms_delete") {
		c.disarmDelete(cc.Name())
	}
	if has("btn_back") {
		c.showWorldTools(cc)
		return
	}
	if has("btn_close") || has("quit") {
		return
	}
	setDraft := func(d *missionDraft) {
		c.mu.Lock()
		c.runtime.missionDrafts[cc.Name()] = d
		c.mu.Unlock()
	}

	// Actions on an existing mission.
	if view != 0 && (has("ms_delete") || has("ms_chest") || has("ms_edit") || has("ms_retools") || has("ms_cancel_edit")) {
		m, err := c.getMissionByID(view)
		if err != nil || m == nil || m.InstanceID != inst.ID {
			c.notify(cc, "The mission no longer exists.")
			c.showWorldTools(cc)
			return
		}
		switch {
		case has("ms_delete"):
			if !c.armDelete(cc.Name(), fmt.Sprintf("mission:%d", m.ID)) {
				break
			}
			c.deleteMission(inst, m.ID)
			if m.global() {
				c.notify(cc, "Mission deleted.")
				c.showWorldTools(cc)
			} else {
				c.notify(cc, "Mission deleted. You can now create a new one.")
				c.openMission(cc, inst, 0, newMissionDraft(m.ZoneID))
			}
			return
		case has("ms_chest"):
			c.giveDeliveryChest(cc, m)
			return
		case has("ms_cancel_edit"):
			setDraft(nil)
		case c.missionComplete(m):
			c.notify(cc, "The mission is completed: it can no longer be changed.")
		case has("ms_retools"):
			c.sendToPlayerServer(cc.Name(), map[string]interface{}{"action": "mission_reset_tools", "mission": m.ID})
			if m.global() {
				c.notify(cc, "Students will get the tools again while they are in this world.")
			} else {
				c.notify(cc, "Students will get the tools again when they are in the zone.")
			}
		case has("ms_edit"):
			setDraft(draftFromMission(m))
		}
		c.showMissionEditor(cc)
		return
	}
	if draft == nil {
		c.showMissionEditor(cc)
		return
	}

	catalog, _ := c.getCatalog(inst.ID)

	c.mu.Lock()
	if v, ok := fm["ms_title"]; ok {
		draft.Title = v
	}
	if v, ok := fm["ms_desc"]; ok {
		draft.Description = v
	}
	if v, ok := fm["ms_count"]; ok {
		draft.Count = parseAmount(v, draft.Count, maxGoalCount)
	}
	c.mu.Unlock()

	var message string
	for k := range fm {
		c.mu.Lock()
		switch {
		case k == "ms_mode_goals":
			draft.Mode = "goals"
		case k == "ms_mode_tools":
			draft.Mode = "tools"
		case strings.HasPrefix(k, "ms_type_"):
			if i, err := strconv.Atoi(strings.TrimPrefix(k, "ms_type_")); err == nil && i != draft.TypeIndex {
				draft.TypeIndex, draft.Target = i, ""
			}
		case strings.HasPrefix(k, "ms_pick_"):
			draft.Target = strings.TrimPrefix(k, "ms_pick_")
		case strings.HasPrefix(k, "ms_amt_"):
			delta := map[string]int{"m10": -10, "m1": -1, "p1": 1, "p10": 10}[strings.TrimPrefix(k, "ms_amt_")]
			draft.Count = min(max(draft.Count+delta, 1), maxGoalCount)
		case strings.HasPrefix(k, "ms_toolamt_"):
			if n, err := strconv.Atoi(strings.TrimPrefix(k, "ms_toolamt_")); err == nil {
				draft.ToolAmount = n
			}
		case strings.HasPrefix(k, "ms_tool_rm_"):
			if i, err := strconv.Atoi(strings.TrimPrefix(k, "ms_tool_rm_")); err == nil && i >= 0 && i < len(draft.Tools) {
				draft.Tools = append(draft.Tools[:i], draft.Tools[i+1:]...)
			}
		case strings.HasPrefix(k, "ms_tool_"):
			if e := findEntry(catalog["tools"], strings.TrimPrefix(k, "ms_tool_")); e != nil {
				var ok bool
				draft.Tools, ok = addTool(draft.Tools, missionTool{Key: e.Key, Label: e.Label, Icon: e.Icon,
					Count: max(draft.ToolAmount, 1)}, maxToolCount)
				if !ok {
					message = fmt.Sprintf("At most %d kinds of tools.", maxMissionTools)
				}
			}
		case strings.HasPrefix(k, "ms_rm_"):
			if i, err := strconv.Atoi(strings.TrimPrefix(k, "ms_rm_")); err == nil && i >= 0 && i < len(draft.Goals) {
				draft.Goals = append(draft.Goals[:i], draft.Goals[i+1:]...)
			}
		case k == "ms_add":
			goalType := missionGoalTypes[clampIndex(draft.TypeIndex, len(missionGoalTypes))-1][0]
			e := findEntry(catalog[catalogKind(goalType)], draft.Target)
			if e == nil {
				message = "Pick what students must deliver, gather, keep or build first."
				break
			}
			var ok bool
			draft.Goals, ok = addGoal(draft.Goals, missionGoal{Type: goalType, Key: e.Key, Label: e.Label,
				Icon: e.Icon, Count: max(draft.Count, 1)}, maxGoalCount)
			if !ok {
				message = fmt.Sprintf("A mission can have at most %d goals.", maxMissionGoals)
			}
		}
		c.mu.Unlock()
	}
	if message != "" {
		c.notify(cc, message)
	}

	if has("ms_start") && draft.EditingID != 0 {
		ok, msg := c.updateMission(inst, draft)
		c.notify(cc, msg)
		if ok {
			setDraft(nil)
		}
	} else if has("ms_start") {
		ok, msg, id := c.createMission(inst, draft)
		c.notify(cc, msg)
		if ok {
			c.mu.Lock()
			c.runtime.missionView[cc.Name()] = id
			c.runtime.missionDrafts[cc.Name()] = nil
			c.mu.Unlock()
			if m, _ := c.getMissionByID(id); m != nil && needsChest(m.Goals, m.global()) {
				c.giveDeliveryChest(cc, m)
			}
		}
	}
	c.showMissionEditor(cc)
}

// giveDeliveryChest gives the teacher a delivery chest; for world missions
// the chest is bound to the mission, zone missions bind it where it's placed.
func (c *controller) giveDeliveryChest(cc *proxy.ClientConn, m *missionData) {
	msg := map[string]interface{}{"action": "give_delivery_chest", "player": cc.Name(), "title": m.Title, "ref": m.ID}
	color := worldMissionColor
	if m.global() {
		msg["mission"] = m.ID
		if inst := c.currentManagedWorld(cc); inst != nil && inst.ClassID != nil && m.GroupID != 0 {
			if g, _ := c.getGroup(*inst.ClassID, m.GroupID); g != nil {
				color = g.Color
			}
		}
	} else if inst := c.currentManagedWorld(cc); inst != nil {
		// The chest item names the zone and takes the zone's color.
		if z := c.zoneByID(inst, m.ZoneID); z != nil {
			msg["zone"] = z.Name
			color = zoneTeachersColor
			if z.Open {
				color = zoneOpenColor
			} else if z.GroupID.Valid && inst.ClassID != nil {
				if g, _ := c.getGroup(*inst.ClassID, int(z.GroupID.Int64)); g != nil {
					color = g.Color
				}
			}
		}
	}
	msg["color"] = color
	goals := make([]map[string]interface{}, 0, len(m.Goals))
	for _, g := range m.Goals {
		goals = append(goals, map[string]interface{}{"type": g.Type, "label": g.Label})
	}
	msg["objectives"] = goals
	c.sendToPlayerServer(cc.Name(), msg)
}

func parseAmount(v string, fallback, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

// ── World Tools → Missions tab ──────────────────────────────────────────────

// newMissionPlaces lists where a new mission can go: the whole world, or a
// zone without a mission yet.
func newMissionPlaces(zones []zoneData, byZone map[int]*missionData) (labels []string, zoneIDs []int) {
	labels, zoneIDs = []string{"Whole world"}, []int{0}
	for _, z := range zones {
		if byZone[z.ID] == nil {
			labels = append(labels, "Zone: "+z.Name)
			zoneIDs = append(zoneIDs, z.ID)
		}
	}
	return labels, zoneIDs
}

func audienceOptions(groups []classGroup) []string {
	out := []string{"Whole class"}
	for _, g := range groups {
		out = append(out, "Group "+g.Name)
	}
	return out
}

func (c *controller) writeMissionsTab(b *strings.Builder, player string, inst *instanceData, zones []zoneData,
	groups []classGroup, groupByID map[int64]classGroup) {
	list, err := c.getWorldMissionList(inst.ID)
	if err != nil {
		b.WriteString(hint(0.55, 2.5, "Could not load the missions."))
		return
	}
	byZone := map[int]*missionData{}
	for i := range list {
		if !list[i].global() {
			byZone[list[i].ZoneID] = &list[i]
		}
	}

	// New mission.
	b.WriteString(box(0.3, 2.1, 10.4, 1.95, colorCard))
	b.WriteString(sectionTitle(0.55, 2.4, "New mission"))
	b.WriteString(hint(2.75, 2.4, "Goals to reach, with tools for the students"))
	places, _ := newMissionPlaces(zones, byZone)
	audiences := audienceOptions(groups)
	c.mu.RLock()
	pick := c.runtime.newMissionPick[player]
	c.mu.RUnlock()
	if pick[0] < 1 || pick[0] > len(places) {
		pick[0] = 1
	}
	if pick[1] < 1 || pick[1] > len(audiences) {
		pick[1] = 1
	}
	b.WriteString(hint(0.55, 2.85, "Where"))
	b.WriteString(fmt.Sprintf("dropdown[0.55,3.05;3.6,0.6;wm_where;%s;%d;true]", dropdownItems(places), pick[0]))
	b.WriteString(tooltip("wm_where", "Whole world: anywhere in this world. Zone: only inside that zone"))
	b.WriteString(hint(4.35, 2.85, "Played by (whole world)"))
	b.WriteString(fmt.Sprintf("dropdown[4.35,3.05;3.3,0.6;wm_group;%s;%d;true]", dropdownItems(audiences), pick[1]))
	b.WriteString(tooltip("wm_group", "Zone missions are played by the zone's group, or the class"))
	b.WriteString(styledBtn(7.85, 3.05, 2.65, 0.6, "wm_create", "+ Create mission", colorPrimary))

	// Missions of this world.
	b.WriteString(box(0.3, 4.25, 10.4, 7.3, colorCard))
	b.WriteString(sectionTitle(0.55, 4.55, fmt.Sprintf("Missions in this world (%d)", len(list))))
	if len(list) == 0 {
		b.WriteString(hint(0.6, 5.1, "No missions yet."))
		return
	}
	zoneNames := map[int]zoneData{}
	for _, z := range zones {
		zoneNames[z.ID] = z
	}
	const top, height, step = 4.85, 6.55, 1.1
	b.WriteString(scrollbarFor("scr_wm_list", 10.35, top, height, len(list), step, 0.05))
	b.WriteString(fmt.Sprintf("scroll_container[0.45,%g;9.85,%g;scr_wm_list;vertical;0.1]", top, height))
	y := 0.05
	for i := range list {
		m := &list[i]
		summary, sumColor := c.missionSummary(m)
		place, color := "Whole world", colorPrimary
		if m.global() {
			if g, ok := groupByID[int64(m.GroupID)]; m.GroupID != 0 && ok {
				place += " · Group " + g.Name
				color = g.Color
			} else {
				place += " · Whole class"
			}
		} else if z, ok := zoneNames[m.ZoneID]; ok {
			place = "Zone " + z.Name
			color = zoneTeachersColor
			if z.Open {
				color = zoneOpenColor
			} else if g, ok := groupByID[z.GroupID.Int64]; z.GroupID.Valid && ok {
				color = g.Color
			}
		}
		if c.missionComplete(m) {
			color = "#3fb56b"
		}
		title := []rune(m.Title)
		if len(title) > 30 {
			title = append(title[:29], '…')
		}
		id := strconv.Itoa(m.ID)
		b.WriteString(box(0, y, 9.8, 1.0, colorRow))
		b.WriteString(box(0, y, 0.14, 1.0, color))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", y+0.28, fmtEsc(string(title))))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", y+0.7, fmtEsc(
			mcColorize(muted, place)+mcColorize(sumColor, "   "+summary))))
		for i, g := range m.Goals {
			if i >= 3 {
				break
			}
			c.smallIcon(b, 6.85+float64(i)*0.47, y+0.29, 0.42, g.Icon)
		}
		b.WriteString(btn(8.3, y+0.22, 1.4, 0.56, "wm_open_"+id, "Open"))
		b.WriteString(tooltip("wm_open_"+id, "See progress, edit goals and tools, or delete"))
		y += step
	}
	b.WriteString("scroll_container_end[]")
}

// createMissionFromWorldTools opens the editor for a new mission chosen on
// the Missions tab.
func (c *controller) createMissionFromWorldTools(cc *proxy.ClientConn, inst *instanceData, groups []classGroup,
	fm map[string]string) {
	zones, _ := c.getZones(inst.ID)
	byZone, _ := c.getMissionsForWorld(inst.ID)
	_, zoneIDs := newMissionPlaces(zones, byZone)
	c.rememberMissionPick(cc.Name(), fm)
	where, _ := strconv.Atoi(fm["wm_where"])
	if where < 1 || where > len(zoneIDs) {
		where = 1
	}
	c.mu.Lock()
	delete(c.runtime.newMissionPick, cc.Name())
	c.mu.Unlock()
	if zoneID := zoneIDs[where-1]; zoneID != 0 {
		c.openMission(cc, inst, 0, newMissionDraft(zoneID))
		return
	}
	groupID := 0
	if idx, _ := strconv.Atoi(fm["wm_group"]); idx > 1 && idx-1 <= len(groups) {
		groupID = groups[idx-2].ID
	}
	c.openMission(cc, inst, 0, newWorldMissionDraft(groupID))
}

// rememberMissionPick keeps the Missions tab dropdowns across redraws.
func (c *controller) rememberMissionPick(player string, fm map[string]string) {
	where, okWhere := fm["wm_where"]
	group, okGroup := fm["wm_group"]
	if !okWhere && !okGroup {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pick := c.runtime.newMissionPick[player]
	if n, err := strconv.Atoi(where); okWhere && err == nil {
		pick[0] = n
	}
	if n, err := strconv.Atoi(group); okGroup && err == nil {
		pick[1] = n
	}
	c.runtime.newMissionPick[player] = pick
}
