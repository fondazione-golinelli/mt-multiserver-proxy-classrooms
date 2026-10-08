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

	const w = 15.5
	var b strings.Builder
	c.peopleFrameWidth(&b, cc, cls, peopleTabGroups, w)

	// ── Left: groups ──
	b.WriteString(box(0.3, 2.05, 5.3, 7.35, colorCard))
	b.WriteString(sectionTitle(0.5, 2.3, "New group"))
	b.WriteString("field[0.5,2.5;3.3,0.55;grp_new_name;;]")
	b.WriteString("field_close_on_enter[grp_new_name;false]")
	b.WriteString(tooltip("grp_new_name", "e.g. Team A, Builders, Table 3"))
	b.WriteString(styledBtn(3.95, 2.5, 1.45, 0.55, "grp_create", "+ Create", colorPrimary))

	b.WriteString(sectionTitle(0.5, 3.45, fmt.Sprintf("Groups (%d)", len(groups))))
	allColor, noneColor := colorButton, colorButton
	if view.Filter == "all" {
		allColor = colorTabFocus
	}
	if view.Filter == "none" {
		noneColor = colorTabFocus
	}
	b.WriteString(styledBtn(0.5, 3.7, 2.4, 0.5, "grp_view_all", fmt.Sprintf("All (%d)", len(students)), allColor))
	b.WriteString(styledBtn(3.0, 3.7, 2.4, 0.5, "grp_view_none", fmt.Sprintf("No group (%d)", len(students)-grouped), noneColor))

	if len(groups) == 0 {
		b.WriteString(hint(0.6, 4.7, "No groups yet: create one above."))
	}
	b.WriteString(scrollbarAt("scr_groups", 5.3, 4.35, 4.95, len(groups), 1.7, 0.05, view.Scroll["scr_groups"]))
	b.WriteString("scroll_container[0.4,4.35;4.85,4.95;scr_groups;vertical;0.1]")
	gy := 0.05
	for _, g := range groups {
		id := strconv.Itoa(g.ID)
		bg := colorRow
		if view.Filter == "g:"+id {
			bg = colorCurrent
		}
		online := len(onlineOf(g.Members))
		b.WriteString(box(0, gy, 4.8, 1.6, bg))
		b.WriteString(box(0, gy, 0.14, 1.6, g.Color))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", gy+0.25, fmtEsc(mcColorize(g.Color, g.Name))))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", gy+0.55, fmtEsc(mcColorize(muted,
			fmt.Sprintf("%s · %d online", plural(len(g.Members), "student", "students"), online)))))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", gy+0.85, fmtEsc(mcColorize(light, memberPreview(g.Members, 28)))))
		b.WriteString(btn(0.3, gy+1.05, 0.95, 0.45, "grp_view_"+id, "Show"))
		b.WriteString(tooltip("grp_view_"+id, "List only the students of this group"))
		b.WriteString(btn(1.33, gy+1.05, 1.0, 0.45, "grp_gather_"+id, "Bring"))
		b.WriteString(tooltip("grp_gather_"+id, "Teleports the online students of this group to you"))
		freezeLabel, freezeColor := "Freeze", colorButton
		if c.isGroupFrozen(g) {
			freezeLabel, freezeColor = "Unfreeze", colorActive
		}
		b.WriteString(styledBtn(2.41, gy+1.05, 1.5, 0.45, "grp_freeze_"+id, freezeLabel, freezeColor))
		if c.isDeleteArmed(cc.Name(), "group:"+id) {
			b.WriteString(styledBtn(3.95, gy+1.05, 0.75, 0.45, "grp_del_"+id, "Sure?", colorDanger))
		} else {
			b.WriteString(iconBtn(4.25, gy+1.05, 0.45, "grp_del_"+id, iconClose, "Delete group (students stay in the class)"))
		}
		gy += 1.7
	}
	b.WriteString("scroll_container_end[]")

	// ── Right: students ──
	b.WriteString(box(5.8, 2.05, 9.4, 7.35, colorCard))
	b.WriteString(fmt.Sprintf("field[6.0,2.25;4.2,0.6;grp_search;;%s]", fmtEsc(view.Search)))
	b.WriteString("field_close_on_enter[grp_search;false]")
	b.WriteString(tooltip("grp_search", "Search by name, then press Enter"))
	b.WriteString(iconBtn(10.3, 2.25, 0.6, "grp_search_go", iconSearch, "Search"))
	labels, keys := filterOptions(groups, groupEditorFixedFilters)
	b.WriteString(fmt.Sprintf("dropdown[11.0,2.25;3.45,0.6;grp_filter;%s;%d;true]", dropdownItems(labels), indexOf(keys, view.Filter)))
	b.WriteString(iconBtn(14.55, 2.25, 0.6, "grp_filter_clear", iconClose, "Clear search and filter"))

	shown := filterStudents(students, byStudent, view.Filter, view.Search)
	selectedCount := 0
	for _, s := range students {
		if view.Selected[s] {
			selectedCount++
		}
	}
	b.WriteString(hint(6.0, 3.17, fmt.Sprintf("Showing %d of %d  ·  %d selected", len(shown), len(students), selectedCount)))
	b.WriteString(btn(10.6, 2.97, 2.0, 0.45, "grp_sel_all", "Select shown"))
	b.WriteString(btn(12.7, 2.97, 2.3, 0.45, "grp_sel_none", "Clear selection"))

	if len(shown) == 0 {
		msg := "No students match the search or filter."
		if len(students) == 0 {
			msg = "No students in this class yet."
		}
		b.WriteString(hint(6.1, 3.95, msg))
	}
	b.WriteString(scrollbarAt("scr_group_students", 14.9, 3.55, 4.15, len(shown), 0.6, 0.05, view.Scroll["scr_group_students"]))
	b.WriteString("scroll_container[5.95,3.55;8.9,4.15;scr_group_students;vertical;0.1]")
	sy := 0.05
	for _, s := range shown {
		bg := colorRow
		if view.Selected[s] {
			bg = colorCurrent
		}
		b.WriteString(box(0, sy, 8.85, 0.55, bg))
		b.WriteString(fmt.Sprintf("checkbox[0.15,%g;sel_%s;;%t]", sy+0.27, fmtEsc(s), view.Selected[s]))
		dot := muted
		if proxy.Find(s) != nil {
			dot = success
		}
		b.WriteString(statusDot(0.7, sy+0.17, dot))
		b.WriteString(fmt.Sprintf("label[1.05,%g;%s]", sy+0.27, fmtEsc(s)))
		if g, ok := byStudent[s]; ok {
			b.WriteString(box(5.0, sy+0.17, 0.22, 0.22, g.Color))
			b.WriteString(fmt.Sprintf("label[5.35,%g;%s]", sy+0.27, fmtEsc(mcColorize(g.Color, g.Name))))
		} else {
			b.WriteString(fmt.Sprintf("label[5.35,%g;%s]", sy+0.27, fmtEsc(mcColorize(muted, "no group"))))
		}
		sy += 0.6
	}
	b.WriteString("scroll_container_end[]")

	// Bulk move.
	b.WriteString(box(5.95, 7.85, 9.1, 1.4, colorRow))
	b.WriteString(coloredLbl(6.15, 8.25, light, fmt.Sprintf("Move %s to:", plural(selectedCount, "student", "students"))))
	targets := []string{"No group"}
	for _, g := range groups {
		targets = append(targets, g.Name)
	}
	target := view.Target
	if target < 1 || target > len(targets) {
		target = 1
	}
	b.WriteString(fmt.Sprintf("dropdown[9.4,8.0;3.2,0.6;grp_target;%s;%d;true]", dropdownItems(targets), target))
	b.WriteString(styledBtn(12.75, 8.0, 2.15, 0.6, "grp_apply", "Apply", colorPrimary))
	b.WriteString(hint(6.15, 8.95, "Tip: filter or search, then Select shown to move many at once."))

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

	var b strings.Builder
	fsOpen(&b, 11, 9.8)
	fsHeader(&b, 11, "World Tools", inst.Title(), false, true)
	if inst.ClassID != nil {
		b.WriteString(styledBtn(7.4, 0.2, 2.55, 0.6, "wt_groups", "Edit groups", colorButton))
		b.WriteString(tooltip("wt_groups", "Create groups and choose who is in each one"))
	}

	// New zone.
	b.WriteString(box(0.3, 1.3, 10.4, 2.2, colorCard))
	b.WriteString(sectionTitle(0.55, 1.6, "New zone"))
	b.WriteString(hint(2.6, 1.6, "Protect an area or mark a meeting point"))
	b.WriteString("field[0.55,2.2;4.1,0.6;wt_name;Name;]")
	b.WriteString("field_close_on_enter[wt_name;false]")
	options := zoneAccessOptions(groups)
	b.WriteString(hint(4.85, 2.0, "Who can build inside"))
	b.WriteString(fmt.Sprintf("dropdown[4.85,2.2;3.2,0.6;wt_type;%s;1;true]", dropdownItems(options)))
	b.WriteString(fmt.Sprintf("style[wt_create;bgcolor=%s]", colorPrimary))
	b.WriteString("button_exit[8.25,2.2;2.25,0.6;wt_create;Create zone]")
	b.WriteString(tooltip("wt_create", "Fly through blocks and mark the corners in the world"))
	b.WriteString(hint(0.55, 3.15, "You'll fly through blocks and mark two corners and a teleport point."))

	// Zones.
	b.WriteString(box(0.3, 3.7, 10.4, 5.85, colorCard))
	b.WriteString(sectionTitle(0.55, 4.0, fmt.Sprintf("Zones in this world (%d)", len(zones))))
	if len(zones) > 0 {
		b.WriteString(styledBtn(7.6, 3.8, 2.9, 0.5, "wt_show", "Show in the world", colorButton))
		b.WriteString(tooltip("wt_show", "Draws the zone borders around you for a few seconds"))
	}
	listTop := 4.5
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
		b.WriteString(hint(0.55, 4.72, "Teleport group:"))
		b.WriteString(fmt.Sprintf("dropdown[2.85,4.45;3.4,0.55;wt_group;%s;%d;true]", dropdownItems(names), selected))
		listTop = 5.2
	}
	if len(zones) == 0 {
		b.WriteString(hint(0.6, 4.7, "No zones yet: everyone can build everywhere."))
	}
	listH := 9.4 - listTop
	b.WriteString(scrollbarFor("scr_wt_zones", 10.35, listTop, listH, len(zones), 0.85, 0.05))
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
		b.WriteString(box(0, zy, 9.8, 0.77, colorRow))
		b.WriteString(box(0, zy, 0.14, 0.77, color))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.24, fmtEsc(string(name))))
		b.WriteString(fmt.Sprintf("label[0.35,%g;%s]", zy+0.56, fmtEsc(mcColorize(muted, z.size()))))
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
		zy += 0.85
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
