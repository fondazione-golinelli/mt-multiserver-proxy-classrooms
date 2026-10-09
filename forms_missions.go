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
	"animals": "These animals must be inside the zone at the same time.",
	"blocks":  "These blocks must be placed inside the zone.",
}

var toolAmounts = []int{1, 4, 16}

// Short labels for the goal type buttons; goalTypeHints explains each.
var goalTypeButtons = []string{"Bring items", "Keep animals", "Build blocks"}

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

func goalSentence(goalType string, count int, label string) string {
	switch goalType {
	case "deliver":
		return fmt.Sprintf("Deliver %d × %s into the delivery chest", count, label)
	case "animals":
		return fmt.Sprintf("Have %d × %s inside the zone", count, label)
	case "blocks":
		return fmt.Sprintf("Place %d × %s blocks inside the zone", count, label)
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

func (c *controller) showMissionEditor(cc *proxy.ClientConn, zoneID int) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	z := c.zoneByID(inst, zoneID)
	if z == nil {
		c.showWorldTools(cc)
		return
	}
	mission, err := c.getMission(zoneID)
	if err != nil {
		c.notify(cc, "Could not load the mission: "+err.Error())
		return
	}
	draft := c.missionDraftFor(cc.Name())
	editing := mission != nil && draft != nil && draft.ZoneID == zoneID && draft.EditingID == mission.ID &&
		!mission.CompletedAt.Valid
	if mission != nil && !editing {
		var b strings.Builder
		fsOpen(&b, 12, 10.4)
		fsHeader(&b, 12, "Mission · "+z.Name, "Played by: "+c.missionAudience(inst, *z), true, true)
		c.writeMissionStatus(&b, cc, z, mission)
		cc.ShowFormspec("classrooms:mission", b.String())
		return
	}

	if draft == nil || draft.ZoneID != zoneID || (mission == nil && draft.EditingID != 0) {
		draft = newMissionDraft(zoneID)
		c.mu.Lock()
		c.runtime.missionDrafts[cc.Name()] = draft
		c.mu.Unlock()
	}
	catalog, _ := c.getCatalog(inst.ID)

	const w, h = 14.2, 11.2
	var b strings.Builder
	fsOpen(&b, w, h)
	heading := "New mission · "
	if editing {
		heading = "Edit mission · "
	}
	fsHeader(&b, w, heading+z.Name, "Played by: "+c.missionAudience(inst, *z), true, true)

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
		c.writeToolPicker(&b, draft, catalog["tools"])
	} else {
		c.writeGoalBuilder(&b, draft, catalog)
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
		text := []rune(goalText(g))
		if len(text) > 23 {
			text = append(text[:22], '…')
		}
		b.WriteString(fmt.Sprintf("label[10.08,%g;%s]", gy+0.31, fmtEsc(string(text))))
		b.WriteString(iconBtn(13.33, gy+0.13, 0.36, fmt.Sprintf("ms_rm_%d", i), iconClose, "Remove: "+goalText(g)))
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
	hasDeliver := false
	for _, g := range draft.Goals {
		hasDeliver = hasDeliver || g.Type == "deliver"
	}
	if editing {
		b.WriteString(hint(0.35, 10.6, "Progress and tools already given are kept."))
	} else if hasDeliver {
		b.WriteString(fmt.Sprintf("image[0.35,10.38;0.42,0.42;%s]", iconWarning))
		b.WriteString(hint(0.9, 10.6, "After starting you get the delivery chest: place it inside the zone."))
	}
	if editing {
		b.WriteString(btn(8.25, 10.2, 2.4, 0.8, "ms_cancel_edit", "Cancel"))
		b.WriteString(styledBtn(10.8, 10.2, 3.1, 0.8, "ms_start", "Save changes", colorPrimary))
	} else {
		b.WriteString(styledBtn(10.8, 10.2, 3.1, 0.8, "ms_start", "Start mission", colorPrimary))
	}

	cc.ShowFormspec("classrooms:mission", b.String())
}

func (c *controller) writeGoalBuilder(b *strings.Builder, draft *missionDraft, catalog missionCatalog) {
	b.WriteString(sectionTitle(0.55, 4.05, "What must students do?"))
	typeIndex := clampIndex(draft.TypeIndex, len(missionGoalTypes))
	for i := range missionGoalTypes {
		color := colorButton
		if i+1 == typeIndex {
			color = colorTabFocus
		}
		b.WriteString(styledBtn(0.5+float64(i)*2.75, 4.3, 2.65, 0.7, fmt.Sprintf("ms_type_%d", i+1), goalTypeButtons[i], color))
		b.WriteString(tooltip(fmt.Sprintf("ms_type_%d", i+1), missionGoalTypes[i][1]))
	}
	goalType := missionGoalTypes[typeIndex-1][0]
	b.WriteString(hint(0.55, 5.25, goalTypeHints[goalType]))

	entries := catalog[goalType]
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
		b.WriteString(coloredLbl(1.35, 8.92, warning, goalSentence(goalType, draft.Count, selected.Label)))
	}
	b.WriteString(styledBtn(6.15, 9.25, 2.6, 0.5, "ms_add", "+ Add this goal", colorPrimary))
}

func (c *controller) writeToolPicker(b *strings.Builder, draft *missionDraft, tools []catalogEntry) {
	b.WriteString(sectionTitle(0.55, 4.05, "Given once to each student entering the zone"))
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

func (c *controller) writeMissionStatus(b *strings.Builder, cc *proxy.ClientConn, z *zoneData, m *missionData) {
	p, hasProgress := c.getMissionProgress(m.ID)
	complete := m.CompletedAt.Valid || (hasProgress && p.Complete)

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
	needsChest := false
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
		b.WriteString(fmt.Sprintf("label[1.15,%g;%s]", gy+0.17, fmtEsc(goalText(g))))
		progressBar(b, 6.4, gy+0.02, 3.9, min(have, g.Count), g.Count, color)
		b.WriteString(fmt.Sprintf("label[10.45,%g;%s]", gy+0.17, fmtEsc(fmt.Sprintf("%d/%d", min(have, g.Count), g.Count))))
		gy += 0.55
		if g.Type == "deliver" {
			needsChest = true
		}
	}

	b.WriteString(box(0.3, 7.3, 11.4, 1.75, colorCard))
	b.WriteString(sectionTitle(0.55, 7.6, "Tools given in the zone"))
	b.WriteString(hint(0.55, 8.0, toolsText(m.Tools)))
	if needsChest {
		if hasProgress && p.Chest {
			b.WriteString(hint(0.55, 8.6, "Delivery chest: placed."))
		} else {
			b.WriteString(coloredLbl(0.55, 8.6, warning, "Delivery chest: not placed yet."))
		}
		b.WriteString(btn(7.0, 8.35, 4.5, 0.55, "ms_chest", "Give me the delivery chest"))
		b.WriteString(tooltip("ms_chest", "Place it inside this zone: students put the requested items in it"))
	}

	if c.isDeleteArmed(cc.Name(), fmt.Sprintf("mission:%d", z.ID)) {
		b.WriteString(coloredLbl(0.55, 9.65, danger, "Sure? Progress is lost."))
		b.WriteString(styledBtn(8.0, 9.3, 3.7, 0.75, "ms_delete", "Yes, delete mission", colorDanger))
		return
	}
	if !complete {
		b.WriteString(btn(0.3, 9.3, 3.6, 0.75, "ms_edit", "Edit goals & tools"))
		b.WriteString(tooltip("ms_edit", "Change goals and support tools: progress and tools already given stay"))
		b.WriteString(btn(4.05, 9.3, 3.75, 0.75, "ms_retools", "Give tools again"))
		b.WriteString(tooltip("ms_retools", "Every student gets the support tools again the next time they are in the zone"))
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
	draft := c.missionDraftFor(cc.Name())
	if draft == nil {
		c.showWorldTools(cc)
		return
	}
	zoneID := draft.ZoneID
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
	if has("ms_delete") {
		if c.armDelete(cc.Name(), fmt.Sprintf("mission:%d", zoneID)) {
			c.deleteMission(inst, zoneID)
			c.notify(cc, "Mission deleted. You can now create a new one.")
		}
		c.showMissionEditor(cc, zoneID)
		return
	}
	if has("ms_chest") {
		c.sendToPlayerServer(cc.Name(), map[string]string{"action": "give_delivery_chest", "player": cc.Name()})
		return
	}
	if has("ms_edit") || has("ms_retools") {
		m, err := c.getMission(zoneID)
		if err != nil || m == nil || m.CompletedAt.Valid {
			c.notify(cc, "The mission is completed or no longer exists.")
			c.showMissionEditor(cc, zoneID)
			return
		}
		if has("ms_retools") {
			c.sendToPlayerServer(cc.Name(), map[string]interface{}{"action": "mission_reset_tools", "mission": m.ID})
			c.notify(cc, "Students will get the tools again when they are in the zone.")
		} else {
			d := newMissionDraft(zoneID)
			d.EditingID, d.Title, d.Description = m.ID, m.Title, m.Description
			d.Goals = append([]missionGoal(nil), m.Goals...)
			d.Tools = append([]missionTool(nil), m.Tools...)
			c.mu.Lock()
			c.runtime.missionDrafts[cc.Name()] = d
			c.mu.Unlock()
		}
		c.showMissionEditor(cc, zoneID)
		return
	}
	if has("ms_cancel_edit") {
		c.mu.Lock()
		c.runtime.missionDrafts[cc.Name()] = newMissionDraft(zoneID)
		c.mu.Unlock()
		c.showMissionEditor(cc, zoneID)
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
			e := findEntry(catalog[goalType], draft.Target)
			if e == nil {
				message = "Pick what students must deliver, keep or build first."
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
			c.mu.Lock()
			c.runtime.missionDrafts[cc.Name()] = newMissionDraft(zoneID)
			c.mu.Unlock()
		}
	} else if has("ms_start") {
		ok, msg := c.createMission(inst, draft)
		c.notify(cc, msg)
		if ok {
			hasDeliver := false
			for _, g := range draft.Goals {
				hasDeliver = hasDeliver || g.Type == "deliver"
			}
			c.mu.Lock()
			c.runtime.missionDrafts[cc.Name()] = newMissionDraft(zoneID)
			c.mu.Unlock()
			if hasDeliver {
				c.sendToPlayerServer(cc.Name(), map[string]string{"action": "give_delivery_chest", "player": cc.Name()})
			}
		}
	}
	c.showMissionEditor(cc, zoneID)
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
