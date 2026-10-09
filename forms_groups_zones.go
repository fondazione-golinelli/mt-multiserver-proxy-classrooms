package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── People → Groups tab ─────────────────────────────────────────────────────

const peopleTabGroups = "tab_groups"

func dropdownItems(items []string) string {
	escaped := make([]string, len(items))
	for i, item := range items {
		escaped[i] = fmtEsc(item)
	}
	return strings.Join(escaped, ",")
}

// ── Student list state (search, filter, selection, scroll) ────────────────

// studentListView is what a teacher has set up in the student lists of one
// class. Checkboxes only submit the box that was clicked, so the selection
// lives here rather than in the form.
type studentListView struct {
	ClassID     int
	Search      string
	Filter      string // "all", "none", "online" or "g:<group id>"
	Target      int    // 1-based index in the "move to" dropdown
	Selected    map[string]bool
	Scroll      map[string]int
	ClassFilter string // filter of the student list in the class view
	// People → Students search.
	PeopleSearch string
}

func (c *controller) studentList(player string, classID int) *studentListView {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.runtime.studentLists[player]
	if v == nil || v.ClassID != classID {
		v = &studentListView{ClassID: classID, Filter: "all", ClassFilter: "all", Target: 1,
			Selected: map[string]bool{}, Scroll: map[string]int{}}
		c.runtime.studentLists[player] = v
	}
	return v
}

// scrollValue parses a submitted scrollbar ("CHG:12" / "VAL:12").
func scrollValue(v string) (int, bool) {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[i+1:]
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

// filterOptions are the student filters: fixed entries, then one per group.
func filterOptions(groups []classGroup, fixed [][2]string) (labels, keys []string) {
	for _, f := range fixed {
		keys = append(keys, f[0])
		labels = append(labels, f[1])
	}
	for _, g := range groups {
		keys = append(keys, "g:"+strconv.Itoa(g.ID))
		labels = append(labels, "Group: "+g.Name)
	}
	return labels, keys
}

func indexOf(keys []string, key string) int {
	for i, k := range keys {
		if k == key {
			return i + 1
		}
	}
	return 1
}

// filterStudents applies a filter key and a case-insensitive name search.
func filterStudents(students []string, byStudent map[string]classGroup, filter, search string) []string {
	search = strings.ToLower(strings.TrimSpace(search))
	var out []string
	for _, s := range students {
		if search != "" && !strings.Contains(strings.ToLower(s), search) {
			continue
		}
		g, grouped := byStudent[s]
		switch {
		case filter == "none" && grouped:
			continue
		case filter == "online" && proxy.Find(s) == nil:
			continue
		case strings.HasPrefix(filter, "g:") && (!grouped || "g:"+strconv.Itoa(g.ID) != filter):
			continue
		}
		out = append(out, s)
	}
	return out
}

// onlineFirst orders names with online players first, keeping the order
// otherwise.
func onlineFirst(names []string) []string {
	var online, offline []string
	for _, n := range names {
		if proxy.Find(n) != nil {
			online = append(online, n)
		} else {
			offline = append(offline, n)
		}
	}
	return append(online, offline...)
}

func memberPreview(members []string, max int) string {
	if len(members) == 0 {
		return "No students yet"
	}
	text := strings.Join(members, ", ")
	if r := []rune(text); len(r) > max {
		text = string(r[:max-1]) + "…"
	}
	return text
}

// ── People → Groups ─────────────────────────────────────────────────────────

var groupEditorFixedFilters = [][2]string{{"all", "All students"}, {"none", "Without group"}, {"online", "Online now"}}

func (c *controller) showGroupsEditor(cc *proxy.ClientConn, classID int) {
	cls, _ := c.getClassByID(classID)
	if cls == nil || !c.canManageClass(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	groups, err := c.getGroups(classID)
	if err != nil {
		c.notify(cc, "Could not load groups: "+err.Error())
	}
	students, _ := c.getStudents(classID)
	byStudent := groupByStudent(groups)
	view := c.studentList(cc.Name(), classID)
	grouped := 0
	for _, s := range students {
		if _, ok := byStudent[s]; ok {
			grouped++
		}
	}

	var b strings.Builder
	c.peopleFrameSize(&b, cc, cls, peopleTabGroups, peopleWideW, peopleWideH)

	// ── Left: groups ──
	b.WriteString(box(0.3, 2.05, 6.6, 9.25, colorCard))
	b.WriteString(sectionTitle(0.55, 2.35, "New group"))
	b.WriteString("field[0.55,2.6;4.3,0.65;grp_new_name;;]")
	b.WriteString("field_close_on_enter[grp_new_name;false]")
	b.WriteString(tooltip("grp_new_name", "e.g. Team A, Builders, Table 3"))
	b.WriteString(styledBtn(5.0, 2.6, 1.65, 0.65, "grp_create", "+ Create", colorPrimary))

	b.WriteString(sectionTitle(0.55, 3.75, fmt.Sprintf("Groups (%d)", len(groups))))
	allColor, noneColor := colorButton, colorButton
	if view.Filter == "all" {
		allColor = colorTabFocus
	}
	if view.Filter == "none" {
		noneColor = colorTabFocus
	}
	b.WriteString(styledBtn(0.55, 4.0, 2.95, 0.6, "grp_view_all", fmt.Sprintf("All students (%d)", len(students)), allColor))
	b.WriteString(styledBtn(3.65, 4.0, 3.0, 0.6, "grp_view_none", fmt.Sprintf("No group (%d)", len(students)-grouped), noneColor))

	if len(groups) == 0 {
		b.WriteString(hint(0.6, 5.1, "No groups yet: create one above."))
	}
	const groupTop, groupH, groupStep = 4.8, 6.35, 2.2
	b.WriteString(scrollbarAt("scr_groups", 6.55, groupTop, groupH, len(groups), groupStep, 0.05, view.Scroll["scr_groups"]))
	b.WriteString(fmt.Sprintf("scroll_container[0.45,%g;6.05,%g;scr_groups;vertical;0.1]", groupTop, groupH))
	gy := 0.05
	for _, g := range groups {
		id := strconv.Itoa(g.ID)
		bg := colorRow
		if view.Filter == "g:"+id {
			bg = colorCurrent
		}
		online := len(onlineOf(g.Members))
		b.WriteString(box(0, gy, 6.0, 2.1, bg))
		b.WriteString(box(0, gy, 0.16, 2.1, g.Color))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", gy+0.3, fmtEsc(mcColorize(g.Color, g.Name)+
			mcColorize(muted, fmt.Sprintf("   %s · %d online", plural(len(g.Members), "student", "students"), online)))))
		preview := memberPreview(g.Members, 36)
		if len(g.Members) == 0 {
			preview = mcColorize(muted, "Nobody yet: select students and move them here.")
		}
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", gy+0.8, fmtEsc(mcColorize(light, preview))))
		b.WriteString(btn(0.35, gy+1.3, 1.3, 0.55, "grp_view_"+id, "Show"))
		b.WriteString(tooltip("grp_view_"+id, "List only the students of this group"))
		b.WriteString(btn(1.75, gy+1.3, 1.3, 0.55, "grp_gather_"+id, "Bring"))
		b.WriteString(tooltip("grp_gather_"+id, "Teleports the online students of this group to you"))
		freezeLabel, freezeColor := "Freeze", colorButton
		if c.isGroupFrozen(g) {
			freezeLabel, freezeColor = "Unfreeze", colorActive
		}
		b.WriteString(styledBtn(3.15, gy+1.3, 1.7, 0.55, "grp_freeze_"+id, freezeLabel, freezeColor))
		if c.isDeleteArmed(cc.Name(), "group:"+id) {
			b.WriteString(styledBtn(4.95, gy+1.3, 0.95, 0.55, "grp_del_"+id, "Sure?", colorDanger))
		} else {
			b.WriteString(iconBtn(5.35, gy+1.3, 0.55, "grp_del_"+id, iconClose, "Delete group (students stay in the class)"))
		}
		gy += groupStep
	}
	b.WriteString("scroll_container_end[]")

	// ── Right: students ──
	b.WriteString(box(7.1, 2.05, 9.6, 9.25, colorCard))
	b.WriteString(fmt.Sprintf("field[7.35,2.3;4.6,0.65;grp_search;;%s]", fmtEsc(view.Search)))
	b.WriteString("field_close_on_enter[grp_search;false]")
	b.WriteString(tooltip("grp_search", "Search by name, then press Enter"))
	b.WriteString(iconBtn(12.05, 2.3, 0.65, "grp_search_go", iconSearch, "Search"))
	labels, keys := filterOptions(groups, groupEditorFixedFilters)
	b.WriteString(fmt.Sprintf("dropdown[12.8,2.3;3.1,0.65;grp_filter;%s;%d;true]", dropdownItems(labels), indexOf(keys, view.Filter)))
	b.WriteString(iconBtn(16.0, 2.3, 0.65, "grp_filter_clear", iconClose, "Clear search and filter"))

	shown := filterStudents(students, byStudent, view.Filter, view.Search)
	selectedCount := 0
	for _, s := range students {
		if view.Selected[s] {
			selectedCount++
		}
	}
	b.WriteString(hint(7.4, 3.4, fmt.Sprintf("Showing %d of %d  ·  %d selected", len(shown), len(students), selectedCount)))
	b.WriteString(btn(12.25, 3.15, 2.05, 0.5, "grp_sel_all", "Select shown"))
	b.WriteString(btn(14.4, 3.15, 2.25, 0.5, "grp_sel_none", "Clear selection"))

	if len(shown) == 0 {
		msg := "No students match the search or filter."
		if len(students) == 0 {
			msg = "No students in this class yet."
		}
		b.WriteString(hint(7.45, 4.25, msg))
	}
	const listTop, listH, rowStep = 3.85, 5.45, 0.7
	b.WriteString(scrollbarAt("scr_group_students", 16.35, listTop, listH, len(shown), rowStep, 0.05, view.Scroll["scr_group_students"]))
	b.WriteString(fmt.Sprintf("scroll_container[7.25,%g;9.0,%g;scr_group_students;vertical;0.1]", listTop, listH))
	sy := 0.05
	for _, s := range shown {
		bg := colorRow
		if view.Selected[s] {
			bg = colorCurrent
		}
		b.WriteString(box(0, sy, 8.95, 0.63, bg))
		b.WriteString(fmt.Sprintf("checkbox[0.2,%g;sel_%s;;%t]", sy+0.31, fmtEsc(s), view.Selected[s]))
		dot := muted
		if proxy.Find(s) != nil {
			dot = success
		}
		b.WriteString(statusDot(0.8, sy+0.21, dot))
		b.WriteString(fmt.Sprintf("label[1.2,%g;%s]", sy+0.31, fmtEsc(s)))
		if g, ok := byStudent[s]; ok {
			b.WriteString(box(5.3, sy+0.2, 0.24, 0.24, g.Color))
			b.WriteString(fmt.Sprintf("label[5.7,%g;%s]", sy+0.31, fmtEsc(mcColorize(g.Color, g.Name))))
		} else {
			b.WriteString(fmt.Sprintf("label[5.7,%g;%s]", sy+0.31, fmtEsc(mcColorize(muted, "no group"))))
		}
		sy += rowStep
	}
	b.WriteString("scroll_container_end[]")

	// Bulk move.
	b.WriteString(box(7.25, 9.5, 9.3, 1.65, colorRow))
	b.WriteString(coloredLbl(7.5, 9.9, light, fmt.Sprintf("Move %s to:", plural(selectedCount, "student", "students"))))
	targets := []string{"No group"}
	for _, g := range groups {
		targets = append(targets, g.Name)
	}
	target := view.Target
	if target < 1 || target > len(targets) {
		target = 1
	}
	b.WriteString(fmt.Sprintf("dropdown[10.75,9.62;3.3,0.6;grp_target;%s;%d;true]", dropdownItems(targets), target))
	b.WriteString(styledBtn(14.2, 9.62, 2.2, 0.6, "grp_apply", "Apply", colorPrimary))
	b.WriteString(hint(7.5, 10.75, "Tip: filter or search, then Select shown to move many at once."))

	cc.ShowFormspec("classrooms:groups", b.String())
}

func (c *controller) handleGroupsEditor(cc *proxy.ClientConn, fields []mt.Field) {
	classID, ok := c.getActiveClass(cc.Name())
	if !ok || !c.canManageClass(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)
	if !hasPrefixKey(fm, "grp_del_") {
		c.disarmDelete(cc.Name())
	}
	if _, ok := fm["btn_back"]; ok {
		if c.getActiveClassOrigin(cc.Name()) == viewOriginWorldTools {
			c.showWorldTools(cc)
			return
		}
		c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
		return
	}
	if c.handlePeopleTabs(cc, fm, classID) {
		return
	}
	if _, ok := fm["btn_close"]; ok {
		return
	}
	if _, ok := fm["quit"]; ok {
		return
	}

	groups, err := c.getGroups(classID)
	if err != nil {
		c.notify(cc, "Could not load groups: "+err.Error())
		return
	}
	students, _ := c.getStudents(classID)
	byStudent := groupByStudent(groups)
	view := c.studentList(cc.Name(), classID)

	// Fields sent with every submit: scroll positions, search, dropdowns.
	c.mu.Lock()
	for _, name := range []string{"scr_groups", "scr_group_students"} {
		if v, ok := scrollValue(fm[name]); ok {
			view.Scroll[name] = v
		}
	}
	if v, ok := fm["grp_search"]; ok {
		view.Search = v
	}
	_, keys := filterOptions(groups, groupEditorFixedFilters)
	if idx, err := strconv.Atoi(fm["grp_filter"]); err == nil && idx >= 1 && idx <= len(keys) {
		view.Filter = keys[idx-1]
	}
	if idx, err := strconv.Atoi(fm["grp_target"]); err == nil {
		view.Target = idx
	}
	// Checkboxes: only the clicked one is submitted.
	for k, v := range fm {
		if strings.HasPrefix(k, "sel_") {
			name := strings.TrimPrefix(k, "sel_")
			if v == "true" {
				view.Selected[name] = true
			} else {
				delete(view.Selected, name)
			}
		}
	}
	c.mu.Unlock()

	findGroup := func(idStr string) *classGroup {
		id, _ := strconv.Atoi(idStr)
		for i := range groups {
			if groups[i].ID == id {
				return &groups[i]
			}
		}
		return nil
	}
	setFilter := func(filter string, clearSearch bool) {
		c.mu.Lock()
		view.Filter = filter
		if clearSearch {
			view.Search = ""
		}
		view.Scroll["scr_group_students"] = 0
		c.mu.Unlock()
	}

	has := func(k string) bool { _, ok := fm[k]; return ok }
	switch {
	case has("grp_create") || fm["key_enter_field"] == "grp_new_name":
		_, msg := c.createGroup(classID, fm["grp_new_name"])
		c.notify(cc, msg)
	case has("grp_view_all"):
		setFilter("all", false)
	case has("grp_view_none"):
		setFilter("none", false)
	case has("grp_filter_clear"):
		setFilter("all", true)
	case has("grp_sel_all"):
		c.mu.Lock()
		for _, s := range filterStudents(students, byStudent, view.Filter, view.Search) {
			view.Selected[s] = true
		}
		c.mu.Unlock()
	case has("grp_sel_none"):
		c.mu.Lock()
		view.Selected = map[string]bool{}
		c.mu.Unlock()
	case has("grp_apply"):
		c.applyGroupMove(cc, classID, groups, byStudent, view)
	default:
		for k := range fm {
			switch {
			case strings.HasPrefix(k, "grp_view_"):
				if g := findGroup(strings.TrimPrefix(k, "grp_view_")); g != nil {
					setFilter("g:"+strconv.Itoa(g.ID), true)
				}
			case strings.HasPrefix(k, "grp_gather_"):
				if g := findGroup(strings.TrimPrefix(k, "grp_gather_")); g != nil {
					c.gatherPlayers(onlineOf(g.Members), cc.Name())
				}
			case strings.HasPrefix(k, "grp_freeze_"):
				if g := findGroup(strings.TrimPrefix(k, "grp_freeze_")); g != nil {
					c.toggleGroupFreeze(*g)
				}
			case strings.HasPrefix(k, "grp_del_"):
				idStr := strings.TrimPrefix(k, "grp_del_")
				if g := findGroup(idStr); g != nil && c.armDelete(cc.Name(), "group:"+idStr) {
					c.deleteGroup(classID, g.ID)
					c.notify(cc, "Group "+g.Name+" deleted. Its zones were removed too.")
					c.mu.Lock()
					if view.Filter == "g:"+idStr {
						view.Filter = "all"
					}
					c.mu.Unlock()
				}
			}
		}
	}
	c.showGroupsEditor(cc, classID)
}

// applyGroupMove moves every selected student to the chosen group.
func (c *controller) applyGroupMove(cc *proxy.ClientConn, classID int, groups []classGroup, byStudent map[string]classGroup, view *studentListView) {
	c.mu.Lock()
	target := view.Target
	selected := make([]string, 0, len(view.Selected))
	for s := range view.Selected {
		selected = append(selected, s)
	}
	c.mu.Unlock()
	if len(selected) == 0 {
		c.notify(cc, "Select some students first.")
		return
	}
	groupID, label := 0, "no group"
	if target >= 2 && target-2 < len(groups) {
		groupID, label = groups[target-2].ID, groups[target-2].Name
	}
	moved := 0
	for _, s := range selected {
		current := 0
		if g, ok := byStudent[s]; ok {
			current = g.ID
		}
		if current == groupID {
			continue
		}
		if err := c.setStudentGroup(classID, s, groupID); err != nil {
			c.notify(cc, "Could not move "+s+": "+err.Error())
			continue
		}
		moved++
	}
	c.mu.Lock()
	view.Selected = map[string]bool{}
	c.mu.Unlock()
	if moved > 0 {
		c.pushZonesForClass(classID)
	}
	c.notify(cc, fmt.Sprintf("Moved %s to %s.", plural(moved, "student", "students"), label))
}

// ── World Tools (third classroom item) ──────────────────────────────────────
//
// Opened by the World Tools item inside a world. Manages the world's zones:
// starting the in-world zone editor, teleporting to zones, showing and
// deleting them.

// currentManagedWorld returns the world the teacher stands in, if they may
// manage it.
func (c *controller) currentManagedWorld(cc *proxy.ClientConn) *instanceData {
	inst, err := c.getInstanceByProxyName(cc.ServerName())
	if err != nil || inst == nil || !c.canManageInstance(inst, cc.Name()) {
		return nil
	}
	return inst
}

func (c *controller) showWorldTools(cc *proxy.ClientConn) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	zones, err := c.getZones(inst.ID)
	if err != nil {
		c.notify(cc, "Could not load zones: "+err.Error())
		return
	}
	var groups []classGroup
	if inst.ClassID != nil {
		groups, _ = c.getGroups(*inst.ClassID)
	}
	groupByID := map[int64]classGroup{}
	for _, g := range groups {
		groupByID[int64(g.ID)] = g
	}

	c.mu.RLock()
	tab := c.runtime.worldToolsTab[cc.Name()]
	c.mu.RUnlock()
	if tab != "missions" {
		tab = "zones"
	}

	var b strings.Builder
	fsOpen(&b, 11, 11.85)
	fsHeader(&b, 11, "World Tools", inst.Title(), false, true)
	b.WriteString(styledBtn(5.75, 0.2, 1.5, 0.6, "wt_map", "Map", colorPrimary))
	b.WriteString(tooltip("wt_map", "World map: zones, waypoints and teleport points for students"))
	if inst.ClassID != nil {
		b.WriteString(styledBtn(7.4, 0.2, 2.55, 0.6, "wt_groups", "Edit groups", colorButton))
		b.WriteString(tooltip("wt_groups", "Create groups and choose who is in each one"))
	}
	tabBar(&b, 0.3, 1.3, 3.0, "wt_tab_"+tab, [][2]string{
		{"wt_tab_zones", fmt.Sprintf("Zones (%d)", len(zones))},
		{"wt_tab_missions", "Missions"},
	})
	if tab == "missions" {
		c.writeMissionsTab(&b, cc.Name(), inst, zones, groups, groupByID)
		cc.ShowFormspec("classrooms:world_tools", b.String())
		return
	}

	// New zone.
	b.WriteString(box(0.3, 2.1, 10.4, 2.2, colorCard))
	b.WriteString(sectionTitle(0.55, 2.4, "New zone"))
	b.WriteString(hint(2.6, 2.4, "Protect an area or mark a meeting point"))
	b.WriteString("field[0.55,3.0;4.1,0.6;wt_name;Name;]")
	b.WriteString("field_close_on_enter[wt_name;false]")
	options := zoneAccessOptions(groups)
	b.WriteString(hint(4.85, 2.8, "Who can build inside"))
	b.WriteString(fmt.Sprintf("dropdown[4.85,3.0;3.2,0.6;wt_type;%s;1;true]", dropdownItems(options)))
	b.WriteString(fmt.Sprintf("style[wt_create;bgcolor=%s]", colorPrimary))
	b.WriteString("button_exit[8.25,3.0;2.25,0.6;wt_create;Create zone]")
	b.WriteString(tooltip("wt_create", "Fly through blocks and mark the corners in the world"))
	b.WriteString(hint(0.55, 3.95, "You'll fly through blocks and mark two corners and a teleport point."))

	// Zones.
	b.WriteString(box(0.3, 4.5, 10.4, 7.05, colorCard))
	b.WriteString(sectionTitle(0.55, 4.8, fmt.Sprintf("Zones in this world (%d)", len(zones))))
	if len(zones) > 0 {
		b.WriteString(styledBtn(7.6, 4.6, 2.9, 0.5, "wt_show", "Show in the world", colorButton))
		b.WriteString(tooltip("wt_show", "Draws the zone borders around you for a few seconds"))
	}
	listTop := 5.3
	if len(groups) > 0 && len(zones) > 0 {
		c.mu.RLock()
		selected := c.runtime.worldToolsGroup[cc.Name()]
		c.mu.RUnlock()
		if selected < 1 || selected > len(groups) {
			selected = 1
		}
		names := make([]string, len(groups))
		for i, g := range groups {
			names[i] = g.Name
		}
		b.WriteString(hint(0.55, 5.52, "Teleport group:"))
		b.WriteString(fmt.Sprintf("dropdown[2.85,5.25;3.4,0.55;wt_group;%s;%d;true]", dropdownItems(names), selected))
		listTop = 6.0
	}
	if len(zones) == 0 {
		b.WriteString(hint(0.6, 5.5, "No zones yet: everyone can build everywhere."))
	}
	listH := 11.4 - listTop
	b.WriteString(scrollbarFor("scr_wt_zones", 10.35, listTop, listH, len(zones), 1.35, 0.05))
	missionsByZone, _ := c.getMissionsForWorld(inst.ID)
	b.WriteString(fmt.Sprintf("scroll_container[0.45,%g;9.85,%g;scr_wt_zones;vertical;0.1]", listTop, listH))
	accessItems := dropdownItems(options)
	zy := 0.05
	for _, z := range zones {
		color := zoneTeachersColor
		if z.Open {
			color = zoneOpenColor
		} else if z.GroupID.Valid {
			if g, ok := groupByID[z.GroupID.Int64]; ok {
				color = g.Color
			}
		}
		id := strconv.Itoa(z.ID)
		name := []rune(z.Name)
		if len(name) > 11 {
			name = append(name[:10], '…')
		}
		b.WriteString(box(0, zy, 9.8, 1.27, colorRow))
		b.WriteString(box(0, zy, 0.14, 1.27, color))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.24, fmtEsc(string(name))))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.56, fmtEsc(mcColorize(muted, z.size()))))
		// Second line: the zone's mission.
		if m := missionsByZone[z.ID]; m != nil {
			summary, sumColor := c.missionSummary(m)
			title := []rune(m.Title)
			if len(title) > 24 {
				title = append(title[:23], '…')
			}
			b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.98, fmtEsc(
				mcColorize(light, "Mission: "+string(title))+mcColorize(sumColor, "   "+summary))))
			b.WriteString(btn(7.62, zy+0.75, 2.08, 0.45, "wt_mission_"+id, "Open mission"))
		} else {
			b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.98, fmtEsc(mcColorize(muted, "No mission"))))
			b.WriteString(styledBtn(7.62, zy+0.75, 2.08, 0.45, "wt_mission_"+id, "+ Mission", colorPrimary))
			b.WriteString(tooltip("wt_mission_"+id, "Give this zone a mission: deliveries, animals or blocks to reach"))
		}
		b.WriteString(fmt.Sprintf("dropdown[2.25,%g;3.5,0.52;wt_access_%s;%s;%d;true]",
			zy+0.13, id, accessItems, zoneAccessIndex(z, groups)))
		b.WriteString(tooltip("wt_access_"+id, "Who can build inside this zone"))
		if _, _, _, _, ok := z.teleportPoint(); ok {
			b.WriteString(iconBtn(5.85, zy+0.13, 0.52, "wt_tp_me_"+id, iconTeleport, "Teleport me here"))
			b.WriteString(btn(6.45, zy+0.13, 1.1, 0.52, "wt_tp_class_"+id, "Class"))
			b.WriteString(tooltip("wt_tp_class_"+id, "Teleport every online student of the class here"))
			if len(groups) > 0 {
				b.WriteString(btn(7.62, zy+0.13, 1.1, 0.52, "wt_tp_group_"+id, "Group"))
				b.WriteString(tooltip("wt_tp_group_"+id, "Teleport the group chosen above here"))
			}
		}
		if c.isDeleteArmed(cc.Name(), "zone:"+id) {
			b.WriteString(styledBtn(8.6, zy+0.13, 1.1, 0.52, "zone_del_"+id, "Sure?", colorDanger))
		} else {
			b.WriteString(iconBtn(9.2, zy+0.13, 0.52, "zone_del_"+id, iconClose, "Remove zone"))
		}
		zy += 1.35
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:world_tools", b.String())
}

func (c *controller) handleWorldTools(cc *proxy.ClientConn, fields []mt.Field) {
	inst := c.currentManagedWorld(cc)
	if inst == nil {
		return
	}
	fm := fieldMap(fields)
	if !hasPrefixKey(fm, "zone_del_") {
		c.disarmDelete(cc.Name())
	}
	var groups []classGroup
	if inst.ClassID != nil {
		groups, _ = c.getGroups(*inst.ClassID)
	}
	if v, ok := fm["wt_group"]; ok {
		if idx, err := strconv.Atoi(v); err == nil {
			c.mu.Lock()
			c.runtime.worldToolsGroup[cc.Name()] = idx
			c.mu.Unlock()
		}
	}

	for _, t := range []string{"zones", "missions"} {
		if _, ok := fm["wt_tab_"+t]; ok {
			c.mu.Lock()
			c.runtime.worldToolsTab[cc.Name()] = t
			c.mu.Unlock()
			c.showWorldTools(cc)
			return
		}
	}
	c.rememberMissionPick(cc.Name(), fm)
	if _, ok := fm["wm_create"]; ok {
		c.createMissionFromWorldTools(cc, inst, groups, fm)
		return
	}
	if k, ok := prefixKey(fm, "wm_open_"); ok {
		if id, err := strconv.Atoi(strings.TrimPrefix(k, "wm_open_")); err == nil {
			c.openMission(cc, inst, id, nil)
		}
		return
	}

	if _, ok := fm["wt_map"]; ok {
		c.sendToPlayerServer(cc.Name(), map[string]string{"action": "open_map", "player": cc.Name()})
		return
	}
	if _, ok := fm["wt_groups"]; ok && inst.ClassID != nil {
		c.setActiveClassWithOrigin(cc.Name(), *inst.ClassID, viewOriginWorldTools)
		c.showGroupsEditor(cc, *inst.ClassID)
		return
	}

	// Access dropdowns are all submitted on every event: apply only changes.
	zones, _ := c.getZones(inst.ID)
	for _, z := range zones {
		v, ok := fm["wt_access_"+strconv.Itoa(z.ID)]
		if !ok {
			continue
		}
		idx, err := strconv.Atoi(v)
		if err != nil || idx == zoneAccessIndex(z, groups) {
			continue
		}
		if access, who, ok := zoneAccessFromIndex(idx, groups); ok {
			if msg := c.updateZoneAccess(inst, z.ID, access); msg != "" {
				c.notify(cc, msg)
			} else {
				c.notify(cc, "Zone "+z.Name+": "+who+".")
			}
		}
	}

	if _, ok := fm["wt_create"]; ok {
		typeIndex, _ := strconv.Atoi(fm["wt_type"])
		access, who, ok := zoneAccessFromIndex(typeIndex, groups)
		if !ok {
			access, who = zoneAccess{}, "Teachers only"
		}
		if !cc.IsModChanJoined(modChannel) {
			go c.ensureChannelJoin(cc)
			c.notify(cc, "The world control channel is still connecting. Try again in a moment.")
			return
		}
		c.startZoneEdit(cc, inst, cleanShortName(fm["wt_name"], 30), access, who)
		return
	}
	if _, ok := fm["btn_close"]; ok {
		return
	}
	if _, ok := fm["quit"]; ok {
		return
	}
	if _, ok := fm["wt_show"]; ok {
		c.pushZones(inst)
		c.sendToPlayerServer(cc.Name(), map[string]string{"action": "show_zones", "player": cc.Name()})
		return
	}

	zones, _ = c.getZones(inst.ID)
	findZone := func(idStr string) *zoneData {
		id, _ := strconv.Atoi(idStr)
		for i := range zones {
			if zones[i].ID == id {
				return &zones[i]
			}
		}
		return nil
	}
	report := func(n int, msg, who string) {
		if msg != "" {
			c.notify(cc, msg)
		} else {
			c.notify(cc, fmt.Sprintf("Teleporting %s (%d).", who, n))
		}
	}

	for k := range fm {
		switch {
		case strings.HasPrefix(k, "wt_tp_me_"):
			if z := findZone(strings.TrimPrefix(k, "wt_tp_me_")); z != nil {
				_, msg := c.teleportToZone(cc, inst, *z, []string{cc.Name()})
				if msg != "" {
					c.notify(cc, msg)
				}
			}
			return
		case strings.HasPrefix(k, "wt_tp_class_"):
			if z := findZone(strings.TrimPrefix(k, "wt_tp_class_")); z != nil && inst.ClassID != nil {
				n, msg := c.teleportToZone(cc, inst, *z, c.getOnlineStudents(*inst.ClassID))
				report(n, msg, "the class")
			}
			c.showWorldTools(cc)
			return
		case strings.HasPrefix(k, "wt_tp_group_"):
			z := findZone(strings.TrimPrefix(k, "wt_tp_group_"))
			c.mu.RLock()
			idx := c.runtime.worldToolsGroup[cc.Name()]
			c.mu.RUnlock()
			if idx < 1 {
				idx = 1
			}
			if z != nil && idx <= len(groups) {
				g := groups[idx-1]
				n, msg := c.teleportToZone(cc, inst, *z, onlineOf(g.Members))
				report(n, msg, "group "+g.Name)
			}
			c.showWorldTools(cc)
			return
		case strings.HasPrefix(k, "wt_mission_"):
			if z := findZone(strings.TrimPrefix(k, "wt_mission_")); z != nil {
				c.openZoneMission(cc, inst, z.ID)
			}
			return
		case strings.HasPrefix(k, "zone_del_"):
			idStr := strings.TrimPrefix(k, "zone_del_")
			if c.armDelete(cc.Name(), "zone:"+idStr) {
				id, _ := strconv.Atoi(idStr)
				c.deleteZone(inst, id)
			}
			c.showWorldTools(cc)
			return
		}
	}

	// Dropdown changes only.
	c.showWorldTools(cc)
}
