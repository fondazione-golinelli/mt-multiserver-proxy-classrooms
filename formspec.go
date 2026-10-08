package main

import (
	"fmt"
	"math"
	"strings"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Shared helpers ──────────────────────────────────────────────────────────

const (
	headerColor = "#1a1a2e"
	accent      = "#e94560"
	light       = "#f0f0f0"
	muted       = "#aaaaaa"
	panel       = "#0f3460"
	success     = "#44FF44"
	danger      = "#FF4444"
	warning     = "#FFCC00"
)

func fmtEsc(s string) string {
	return proxy.FormspecEscape(s)
}

func mcColorize(color, text string) string {
	return "\x1b(c@" + color + ")" + text + "\x1b(c@#)"
}

func btn(x, y, w, h float64, name, label string) string {
	return fmt.Sprintf("button[%g,%g;%g,%g;%s;%s]", x, y, w, h, name, fmtEsc(label))
}

func btnExit(x, y, w, h float64, name, label string) string {
	return fmt.Sprintf("button_exit[%g,%g;%g,%g;%s;%s]", x, y, w, h, name, fmtEsc(label))
}

func coloredLbl(x, y float64, color, text string) string {
	return fmt.Sprintf("label[%g,%g;%s]", x, y, fmtEsc(mcColorize(color, text)))
}

func box(x, y, w, h float64, color string) string {
	return fmt.Sprintf("box[%g,%g;%g,%g;%s]", x, y, w, h, color)
}

func checkbox(x, y float64, name, label string, checked bool) string {
	return fmt.Sprintf("checkbox[%g,%g;%s;%s;%t]", x, y, fmtEsc(name), fmtEsc(label), checked)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var classViewFixedFilters = [][2]string{{"all", "All students"}, {"online", "Online now"}, {"none", "Without group"}}

// ── Panel entry point ──────────────────────────────────────────────────────

// showPanelHome opens the class of the world the player is in, when that
// world belongs to a class they can see; otherwise the class dashboard.
func (c *controller) showPanelHome(cc *proxy.ClientConn) {
	if inst, err := c.getInstanceByProxyName(cc.ServerName()); err == nil && inst != nil &&
		inst.ClassID != nil && c.canViewClass(*inst.ClassID, cc.Name()) {
		c.showClassView(cc, *inst.ClassID)
		return
	}
	c.showMainDashboard(cc)
}

// ── Main Dashboard (Class List) ─────────────────────────────────────────────

func (c *controller) showMainDashboard(cc *proxy.ClientConn) {
	name := cc.Name()
	isFullTeacher := c.isTeacher(name)
	classes, err := c.getClasses(name)
	if err != nil {
		cc.SendChatMsg("[Classrooms] Error loading classes: " + err.Error())
		return
	}

	var b strings.Builder
	fsOpen(&b, 12, 9)
	subtitle := "Assistance dashboard"
	if isFullTeacher {
		subtitle = "Teacher dashboard"
	}
	fsHeader(&b, 12, "Classrooms", subtitle, false, true)
	if c.isAdmin(name) {
		b.WriteString(iconBtn(10.35, 0.17, 0.66, "btn_admin_panel", iconGear, "Admin panel: all worlds, classes and teachers"))
	}

	listY := 2.0
	if isFullTeacher {
		b.WriteString(box(0.3, 1.3, 11.4, 0.95, colorCard))
		b.WriteString(hint(0.55, 1.78, "New class"))
		b.WriteString("field[1.9,1.48;6.9,0.6;new_class_name;;]")
		b.WriteString("field_close_on_enter[new_class_name;false]")
		b.WriteString(tooltip("new_class_name", "Type a name for the new class, e.g. 3B Science"))
		b.WriteString(styledBtn(9.0, 1.48, 2.5, 0.6, "btn_create_class", "Create class", colorPrimary))
		listY = 2.75
	}
	b.WriteString(sectionTitle(0.35, listY, "Your classes"))

	if len(classes) == 0 {
		emptyText := "No classes are assigned to you yet."
		if isFullTeacher {
			emptyText = "No classes yet: type a name above and press Create class."
		}
		b.WriteString(hint(0.5, listY+0.8, emptyText))
		cc.ShowFormspec("classrooms:main", b.String())
		return
	}

	listY += 0.35
	listH := 8.8 - listY
	b.WriteString(scrollbarFor("scr_classes", 11.45, listY, listH, len(classes), 1.15, 0.05))
	b.WriteString(fmt.Sprintf("scroll_container[0.3,%g;11.05,%g;scr_classes;vertical;0.1]", listY, listH))
	y := 0.05
	for _, cls := range classes {
		students, _ := c.getStudents(cls.ID)
		online := 0
		for _, s := range students {
			if proxy.Find(s) != nil {
				online++
			}
		}
		worldsOnline := 0
		if instances, err := c.getInstancesForClass(cls.ID); err == nil {
			for _, inst := range instances {
				if inst.Status == "running" {
					worldsOnline++
				}
			}
		}

		b.WriteString(box(0, y, 11.0, 1.05, colorCard))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", y+0.32, fmtEsc(mcColorize(light, cls.Name))))
		dot := muted
		if online > 0 {
			dot = success
		}
		b.WriteString(statusDot(0.3, y+0.66, dot))
		b.WriteString(fmt.Sprintf("label[0.65,%g;%s]", y+0.77, fmtEsc(mcColorize(muted,
			fmt.Sprintf("%d/%s online   ·   %s running", online, plural(len(students), "student", "students"),
				plural(worldsOnline, "world", "worlds"))))))

		b.WriteString(styledBtn(7.55, y+0.22, 1.9, 0.62, fmt.Sprintf("open_class_%d", cls.ID), "Open", colorPrimary))
		if cls.CreatedBy == name || c.isAdmin(name) {
			delName := fmt.Sprintf("del_class_%d", cls.ID)
			if c.isDeleteArmed(name, fmt.Sprintf("class:%d", cls.ID)) {
				b.WriteString(styledBtn(9.55, y+0.22, 1.3, 0.62, delName, "Delete?", colorDanger))
				b.WriteString(fmt.Sprintf("tooltip[%s;Click again to delete this class permanently]", delName))
			} else {
				b.WriteString(iconBtn(10.2, y+0.22, 0.62, delName, iconClose, "Delete class"))
			}
		}
		y += 1.15
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:main", b.String())
}

// ── Class View (Students + Worlds) ──────────────────────────────────────────

func (c *controller) showClassView(cc *proxy.ClientConn, classID int) {
	c.showClassViewWithOrigin(cc, classID, viewOriginTeacher)
}

func (c *controller) showClassViewWithOrigin(cc *proxy.ClientConn, classID int, origin string) {
	if origin == "" {
		origin = viewOriginTeacher
	}
	cls, err := c.getClassByID(classID)
	if err != nil || cls == nil || !c.canViewClass(classID, cc.Name()) {
		c.showClassFallback(cc, origin)
		return
	}
	c.setActiveClassWithOrigin(cc.Name(), classID, origin)

	students, _ := c.getStudents(classID)
	canManage := c.canManageClass(classID, cc.Name())
	instances, _ := c.getInstancesForClass(classID)
	online := 0
	for _, s := range students {
		if proxy.Find(s) != nil {
			online++
		}
	}

	var b strings.Builder
	fsOpen(&b, 16, 9.6)
	subtitle := fmt.Sprintf("%d/%s online", online, plural(len(students), "student", "students"))
	if origin == viewOriginAdminClasses {
		subtitle += "   ·   Owner: " + cls.CreatedBy
	}
	fsHeader(&b, 16, cls.Name, subtitle, true, true)

	// Left column: live controls and students.
	b.WriteString(box(0.2, 1.3, 8.6, 8.1, colorCard))
	listY := 2.35
	if canManage {
		b.WriteString(sectionTitle(0.45, 1.6, "Live controls (online students)"))
		frozen := c.isClassFrozen(classID)
		watching := c.isClassWatching(classID, cc.Name())
		freezeCaption := "Freeze"
		if frozen {
			freezeCaption = "Unfreeze"
		}
		watchCaption := "Look at me"
		if watching {
			watchCaption = "Stop looking"
		}
		actionTile(&b, 0.45, 1.95, 2.6, "btn_toggle_freeze", iconFreeze, freezeCaption,
			"Stops every online student from moving. Click again to release them.", frozen)
		actionTile(&b, 3.2, 1.95, 2.6, "btn_gather_all", iconGather, "Bring here",
			"Teleports every online student around you.", false)
		actionTile(&b, 5.95, 1.95, 2.6, "btn_toggle_watch", iconEye, watchCaption,
			"Turns every online student's camera towards you.", watching)
		listY = 4.0
	}

	b.WriteString(sectionTitle(0.45, listY, "Students"))
	b.WriteString(styledBtn(5.55, listY-0.3, 3.0, 0.58, "btn_people", "Manage people", colorButton))
	b.WriteString(tooltip("btn_people", "Add or remove students, create student accounts, and manage groups, assistance and teachers"))
	groups, _ := c.getGroups(classID)
	byStudent := groupByStudent(groups)
	view := c.studentList(cc.Name(), classID)
	filterLabels, filterKeys := filterOptions(groups, classViewFixedFilters)
	b.WriteString(fmt.Sprintf("dropdown[2.1,%g;3.3,0.58;cls_filter;%s;%d;true]",
		listY-0.3, dropdownItems(filterLabels), indexOf(filterKeys, view.ClassFilter)))
	b.WriteString(tooltip("cls_filter", "Show only some students"))
	shown := onlineFirst(filterStudents(students, byStudent, view.ClassFilter, ""))
	listTop := listY + 0.4
	listH := 9.25 - listTop
	if len(students) == 0 {
		b.WriteString(hint(0.6, listTop+0.4, "No students yet: use Manage people to add them."))
	} else if len(shown) == 0 {
		b.WriteString(hint(0.6, listTop+0.4, "No students match this filter."))
	}
	b.WriteString(scrollbarAt("scr_students", 8.45, listTop, listH, len(shown), 0.7, 0.05, view.Scroll["scr_students"]))
	b.WriteString(fmt.Sprintf("scroll_container[0.4,%g;8.0,%g;scr_students;vertical;0.1]", listTop, listH))
	sy := 0.05
	for _, s := range shown {
		isOnline := proxy.Find(s) != nil
		b.WriteString(box(0, sy, 7.95, 0.62, colorRow))
		dot := muted
		if isOnline {
			dot = success
		}
		b.WriteString(statusDot(0.2, sy+0.2, dot))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", sy+0.31, fmtEsc(s)))
		if g, ok := byStudent[s]; ok {
			name := []rune(g.Name)
			if len(name) > 14 {
				name = append(name[:13], '…')
			}
			b.WriteString(box(3.9, sy+0.2, 0.22, 0.22, g.Color))
			b.WriteString(fmt.Sprintf("label[4.25,%g;%s]", sy+0.31, fmtEsc(mcColorize(g.Color, string(name)))))
		}
		if isOnline {
			b.WriteString(iconBtn(6.55, sy+0.06, 0.5, "tp_to_"+s, iconTeleport, "Teleport to "+s))
			if canManage {
				b.WriteString(iconBtn(7.2, sy+0.06, 0.5, "watch_"+s, iconEye, "Make "+s+" look at you"))
			}
		} else {
			b.WriteString(fmt.Sprintf("label[6.55,%g;%s]", sy+0.31, fmtEsc(mcColorize(muted, "offline"))))
		}
		sy += 0.7
	}
	b.WriteString("scroll_container_end[]")

	// Right column: class worlds.
	b.WriteString(box(9.0, 1.3, 6.8, 8.1, colorCard))
	b.WriteString(sectionTitle(9.25, 1.6, "Class worlds"))
	worlds := instances
	if !canManage {
		worlds = worlds[:0:0]
		for _, inst := range instances {
			if inst.Status == "running" {
				worlds = append(worlds, inst)
			}
		}
	}
	worldTop := 2.0
	var current *instanceData
	for i := range instances {
		if instances[i].ProxyName == cc.ServerName() {
			current = &instances[i]
		}
	}
	if current != nil {
		c.writeCurrentWorldCard(&b, current, canManage)
		worldTop = 4.65
	}
	if canManage {
		b.WriteString(styledBtn(9.25, worldTop-0.05, 6.3, 0.65, "btn_create_instance", "+ New class world", colorPrimary))
		b.WriteString(tooltip("btn_create_instance", "Create a new world from a map template for this class"))
		worldTop += 0.8
	}
	worldBottom := 9.25
	if canManage {
		worldBottom = 8.4
	}
	if len(worlds) == 0 {
		msg := "No class worlds are running right now."
		if canManage {
			msg = "No worlds yet: press + New class world."
		}
		b.WriteString(hint(9.3, worldTop+0.4, msg))
	}
	worldH := worldBottom - worldTop
	b.WriteString(scrollbarFor("scr_instances", 15.45, worldTop, worldH, len(worlds), 1.55, 0.05))
	b.WriteString(fmt.Sprintf("scroll_container[9.2,%g;6.2,%g;scr_instances;vertical;0.1]", worldTop, worldH))
	iy := 0.05
	for _, inst := range worlds {
		rowColor := colorRow
		if current != nil && inst.ID == current.ID {
			rowColor = colorCurrent
		}
		b.WriteString(box(0, iy, 6.15, 1.45, rowColor))
		b.WriteString(statusDot(0.2, iy+0.2, instanceStatusColor(inst.Status)))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.31, fmtEsc(mcColorize(light, inst.Title()))))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.66, fmtEsc(
			mcColorize(instanceStatusColor(inst.Status), instanceStatusLabel(inst.Status))+
				mcColorize(muted, "  ·  "+inst.TemplateName))))
		bx := 0.2
		if inst.Status == "running" {
			here := cc.ServerName() == inst.ProxyName
			if !here {
				b.WriteString(styledBtn(bx, iy+0.93, 1.5, 0.42, "join_inst_"+inst.ID, "Join", colorPrimary))
				b.WriteString(fmt.Sprintf("tooltip[join_inst_%s;Go to this world]", inst.ID))
				bx += 1.6
			} else {
				b.WriteString(fmt.Sprintf("label[%g,%g;%s]", bx, iy+1.14, fmtEsc(mcColorize(success, "You are here"))))
				bx += 2.1
			}
			if canManage {
				b.WriteString(btn(bx, iy+0.93, 2.0, 0.42, "bring_inst_"+inst.ID, "Bring class"))
				b.WriteString(fmt.Sprintf("tooltip[bring_inst_%s;Move you and every online student of this class into this world]", inst.ID))
			}
		}
		if canManage {
			b.WriteString(iconBtn(5.45, iy+0.15, 0.55, "open_inst_"+inst.ID, iconGear, "Manage: start/stop, rules, time & weather"))
		}
		iy += 1.55
	}
	b.WriteString("scroll_container_end[]")
	if canManage {
		b.WriteString(btn(9.25, 8.6, 6.3, 0.6, "btn_return_hub", "Return to HUB"))
	}

	cc.ShowFormspec("classrooms:class", b.String())
}

// onOff renders a boolean rule for the current-world summary.
func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// writeCurrentWorldCard highlights the class world the viewer is standing in,
// with a summary of its rules and shortcuts to change them.
func (c *controller) writeCurrentWorldCard(b *strings.Builder, inst *instanceData, canManage bool) {
	b.WriteString(box(9.2, 1.95, 6.4, 2.5, colorCurrent))
	b.WriteString(box(9.2, 1.95, 0.08, 2.5, colorActive))
	b.WriteString(coloredLbl(9.45, 2.22, warning, "YOU ARE IN THIS WORLD"))
	if c.isRestartPending(inst.ID) {
		b.WriteString(fmt.Sprintf("image[13.2,2.04;0.36,0.36;%s]", iconWarning))
		b.WriteString(coloredLbl(13.65, 2.22, warning, "restart needed"))
	}
	b.WriteString(coloredLbl(9.45, 2.62, light, inst.Title()))

	if settings, err := c.getInstanceSettingsOrDefault(inst.ID); err == nil {
		b.WriteString(hint(9.45, 3.02, fmt.Sprintf("Hurt: %s  ·  PvP: %s  ·  Hunger: %s",
			onOff(settings.EnableDamage), onOff(settings.EnablePVP), onOff(settings.EnableHunger))))
		mobs := "no mobs"
		if settings.MobsSpawn && settings.OnlyPeacefulMobs {
			mobs = "peaceful mobs"
		} else if settings.MobsSpawn {
			mobs = "all mobs"
		}
		timeState := "time flowing"
		if c.isInstanceTimeStopped(inst.ID) {
			timeState = "time paused"
		}
		b.WriteString(hint(9.45, 3.38, "Mobs: "+mobs+"  ·  "+timeState))
	}

	if canManage {
		b.WriteString(styledBtn(9.45, 3.75, 2.4, 0.52, "cur_rules_"+inst.ID, "Rules", colorButton))
		b.WriteString(tooltip("cur_rules_"+inst.ID, "Damage, PvP, hunger, mobs and arrival point of this world"))
		b.WriteString(styledBtn(11.95, 3.75, 2.85, 0.52, "cur_world_"+inst.ID, "Time & weather", colorButton))
		b.WriteString(iconBtn(14.95, 3.75, 0.52, "open_inst_"+inst.ID, iconGear, "Manage this world"))
	}
}

func (c *controller) showClassFallback(cc *proxy.ClientConn, origin string) {
	if origin == viewOriginAdminClasses {
		c.showAdminPanelTab(cc, "classes")
		return
	}
	c.showMainDashboard(cc)
}

// ── Instance Creation (Template Picker) ─────────────────────────────────────

func (c *controller) showTemplatePicker(cc *proxy.ClientConn, classID *int) {
	templates := c.getPublicTemplates()
	if c.isAdmin(cc.Name()) {
		templates = c.getAllTemplates()
	}

	var b strings.Builder
	fsOpen(&b, 11, 8.6)
	fsHeader(&b, 11, "New class world", "Name it, then pick a map", true, true)

	b.WriteString(box(0.3, 1.3, 10.4, 1.15, colorCard))
	b.WriteString(coloredLbl(0.55, 1.6, warning, "1"))
	b.WriteString(coloredLbl(0.85, 1.6, light, "World name"))
	b.WriteString("field[0.55,1.85;9.9,0.5;new_instance_name;;]")
	b.WriteString("field_close_on_enter[new_instance_name;false]")

	b.WriteString(coloredLbl(0.55, 2.85, warning, "2"))
	b.WriteString(coloredLbl(0.85, 2.85, light, "Choose a map"))
	b.WriteString(scrollbarFor("scr_templates", 10.45, 3.15, 4.55, len(templates), 1.05, 0.05))
	b.WriteString("scroll_container[0.3,3.15;10.1,4.55;scr_templates;vertical;0.1]")
	y := 0.05
	for _, tName := range templates {
		tpl := c.cfg.Templates[tName]
		b.WriteString(box(0, y, 10.0, 0.95, colorCard))
		b.WriteString(fmt.Sprintf("image[0.2,%g;0.55,0.55;%s]", y+0.2, iconGlobe))
		b.WriteString(fmt.Sprintf("label[0.95,%g;%s]", y+0.3, fmtEsc(mcColorize(light, tName))))
		b.WriteString(fmt.Sprintf("label[0.95,%g;%s]", y+0.66, fmtEsc(mcColorize(muted, tpl.ServerDescription))))
		b.WriteString(styledBtn(8.0, y+0.17, 1.8, 0.6, "pick_tpl_"+tName, "Create", colorPrimary))
		y += 1.05
	}
	b.WriteString("scroll_container_end[]")

	b.WriteString(fmt.Sprintf("image[0.35,7.9;0.4,0.4;%s]", iconCheck))
	b.WriteString(hint(0.9, 8.1, "New worlds start safe: no damage or hunger, peaceful mobs."))

	cc.ShowFormspec("classrooms:template_picker", b.String())
}

// ── Instance Operation Dialogs ──────────────────────────────────────────────

func dialogFrame(b *strings.Builder, w, h float64, color, icon, title string) {
	fsOpen(b, w, h)
	b.WriteString(box(0, 0, w, 1.1, panel))
	b.WriteString(fmt.Sprintf("image[0.35,0.25;0.6,0.6;%s]", icon))
	b.WriteString(coloredLbl(1.15, 0.55, light, title))
	b.WriteString(box(0, 1.1, w, 0.06, color))
}

func (c *controller) showInstanceProgress(cc *proxy.ClientConn, title, detail string) {
	var b strings.Builder
	dialogFrame(&b, 9, 5.4, warning, iconRefresh, title)
	b.WriteString(box(0.5, 1.6, 8, 2.5, colorCard))
	b.WriteString(coloredLbl(0.85, 2.15, light, detail))
	b.WriteString(hint(0.85, 2.85, "This can take a minute. You can close this window:"))
	b.WriteString(hint(0.85, 3.35, "a new one opens when the world is ready."))
	b.WriteString(btnExit(3.0, 4.4, 3.0, 0.7, "btn_progress_close", "Close"))
	cc.ShowFormspec("classrooms:instance_progress", b.String())
}

func (c *controller) showInstanceReady(cc *proxy.ClientConn, inst *instanceData, title string) {
	if inst == nil || !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	origin := viewOriginAdminInstances
	if inst.ClassID != nil {
		origin = c.getActiveClassOrigin(cc.Name())
		if origin == "" {
			origin = viewOriginTeacher
		}
	}
	c.setActiveInstanceWithOrigin(cc.Name(), inst.ID, origin)

	var b strings.Builder
	dialogFrame(&b, 10, 6.6, success, iconCheck, title)
	b.WriteString(box(0.5, 1.6, 9, 1.8, colorCard))
	b.WriteString(coloredLbl(0.85, 2.1, light, inst.Title()))
	b.WriteString(hint(0.85, 2.7, "Map: "+inst.TemplateName+"   ·   The world is online. What next?"))

	tileW := 2.7
	x := 0.5
	actionTile(&b, x, 3.7, tileW, "btn_ready_hop_me", iconEnter, "Go there", "Join the world yourself", false)
	x += tileW + 0.45
	if inst.ClassID != nil {
		actionTile(&b, x, 3.7, tileW, "btn_ready_hop_class", iconPeople, "Bring class", "Move you and every online student into the world", false)
		x += tileW + 0.45
	}
	actionTile(&b, x, 3.7, tileW, "btn_ready_open", iconGear, "Manage", "Rules, time & weather, start/stop", false)
	b.WriteString(btnExit(3.7, 5.85, 2.6, 0.55, "btn_ready_close", "Close"))

	cc.ShowFormspec("classrooms:instance_ready", b.String())
}

func (c *controller) showInstanceError(cc *proxy.ClientConn, inst *instanceData, title, detail string) {
	if inst != nil {
		c.setActiveInstance(cc.Name(), inst.ID)
	}

	var b strings.Builder
	dialogFrame(&b, 10, 6.6, danger, iconError, title)
	b.WriteString(box(0.5, 1.6, 9, 3.5, colorCard))
	b.WriteString(coloredLbl(0.85, 2.05, light, "The operation did not complete. Details:"))
	b.WriteString(fmt.Sprintf("textarea[0.85,2.4;8.3,2.5;error_detail;;%s]", fmtEsc(detail)))

	if inst != nil {
		b.WriteString(btn(1.0, 5.5, 2.6, 0.75, "btn_error_open", "Manage world"))
	}
	b.WriteString(btnExit(6.4, 5.5, 2.6, 0.75, "btn_error_close", "Close"))

	cc.ShowFormspec("classrooms:instance_error", b.String())
}

// ── World (Instance) View: Overview | Rules | Time & weather ────────────────

const (
	instTabOverview = "tab_overview"
	instTabRules    = "tab_rules"
	instTabWorld    = "tab_world"
)

// instanceFrame draws the shared header and tab bar of the world screens.
func instanceFrame(b *strings.Builder, inst *instanceData, activeTab string) {
	fsOpen(b, 11, 9.6)
	subtitle := instanceStatusLabel(inst.Status) + "   ·   " + inst.TemplateName
	fsHeader(b, 11, inst.Title(), subtitle, true, true)
	tabBar(b, 0.3, 1.3, 3.4, activeTab, [][2]string{
		{instTabOverview, "Overview"},
		{instTabRules, "Rules"},
		{instTabWorld, "Time & weather"},
	})
}

func (c *controller) showInstanceView(cc *proxy.ClientConn, instanceID string) {
	c.showInstanceViewWithOrigin(cc, instanceID, viewOriginTeacher)
}

func (c *controller) showInstanceViewWithOrigin(cc *proxy.ClientConn, instanceID, origin string) {
	if origin == "" {
		origin = viewOriginTeacher
	}
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil || !c.canManageInstance(inst, cc.Name()) {
		c.showInstanceFallback(cc, origin)
		return
	}
	c.setActiveInstanceWithOrigin(cc.Name(), instanceID, origin)

	var b strings.Builder
	instanceFrame(&b, inst, instTabOverview)

	// Main actions.
	b.WriteString(box(0.3, 2.15, 10.4, 2.2, colorCard))
	switch inst.Status {
	case "running":
		here := cc.ServerName() == inst.ProxyName
		joinCaption := "Go there"
		if here {
			joinCaption = "You are here"
		}
		actionTile(&b, 0.5, 2.3, 3.2, "btn_hop_me", iconEnter, joinCaption, "Join this world yourself", here)
		if inst.ClassID != nil {
			actionTile(&b, 3.9, 2.3, 3.2, "btn_hop_class", iconPeople, "Bring class",
				"Move you and every online student of the class into this world", false)
		}
		actionTile(&b, 7.3, 2.3, 3.2, "btn_inst_stop", iconClose, "Stop world",
			"Sends everyone inside back to the lobby and turns the world off. Nothing is lost.", false)
	case "stopped":
		actionTile(&b, 0.5, 2.3, 3.2, "btn_inst_start", iconNext, "Start world",
			"Turns the world on. It takes about a minute.", false)
		b.WriteString(hint(4.0, 2.95, "The world is off. Start it to play;"))
		b.WriteString(hint(4.0, 3.4, "everything built so far is kept."))
	default:
		b.WriteString(fmt.Sprintf("image[0.6,2.6;0.6,0.6;%s]", iconRefresh))
		b.WriteString(coloredLbl(1.4, 2.9, warning, "The world is being created. This takes a minute or two."))
	}

	// Details.
	b.WriteString(box(0.3, 4.55, 10.4, 1.5, colorCard))
	b.WriteString(sectionTitle(0.55, 4.85, "Details"))
	b.WriteString(hint(0.55, 5.3, "Created by: "+inst.CreatedBy))
	if inst.Institute != "" {
		b.WriteString(hint(5.4, 5.3, "Institute: "+inst.Institute))
	}
	b.WriteString(hint(0.55, 5.75, "Join command: /join "+inst.ProxyName))

	// Guests.
	b.WriteString(box(0.3, 6.25, 10.4, 1.4, colorCard))
	b.WriteString(sectionTitle(0.55, 6.55, "Invite a guest"))
	b.WriteString(hint(4.0, 6.55, "Lets a player outside the class join this world"))
	b.WriteString("field[0.55,6.85;7.4,0.6;invite_name;;]")
	b.WriteString("field_close_on_enter[invite_name;false]")
	b.WriteString(tooltip("invite_name", "Player name to invite"))
	b.WriteString(styledBtn(8.15, 6.85, 2.35, 0.6, "btn_invite", "+ Invite", colorPrimary))

	// Danger zone.
	b.WriteString(box(0.3, 7.85, 10.4, 1.5, colorCard))
	b.WriteString(sectionTitle(0.55, 8.15, "Delete"))
	if c.isDeleteArmed(cc.Name(), "instance:"+inst.ID) {
		b.WriteString(coloredLbl(0.55, 8.7, danger, "Sure? This cannot be undone."))
		b.WriteString(styledBtn(7.3, 8.35, 3.2, 0.7, "btn_inst_delete", "Yes, delete forever", colorDanger))
	} else {
		b.WriteString(hint(0.55, 8.7, "Removes the world and all builds."))
		b.WriteString(styledBtn(7.3, 8.35, 3.2, 0.7, "btn_inst_delete", "Delete world", colorDanger))
	}

	cc.ShowFormspec("classrooms:instance", b.String())
}

func settingRow(b *strings.Builder, x, y float64, icon, name, label, help string, checked bool) {
	if icon != "" {
		b.WriteString(fmt.Sprintf("image[%g,%g;0.42,0.42;%s]", x, y-0.21, icon))
	}
	b.WriteString(checkbox(x+0.6, y, name, label, checked))
	b.WriteString(fmt.Sprintf("tooltip[%s;%s]", name, fmtEsc(help)))
}

func (c *controller) showInstanceSettings(cc *proxy.ClientConn, instanceID string) {
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil || !c.canManageInstance(inst, cc.Name()) {
		c.showInstanceFallback(cc, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	c.setActiveInstanceWithOrigin(cc.Name(), instanceID, c.getActiveInstanceOrigin(cc.Name()))
	settings, err := c.getInstanceSettingsOrDefault(instanceID)
	if err != nil {
		c.showInstanceError(cc, inst, "Settings error", err.Error())
		return
	}

	var b strings.Builder
	instanceFrame(&b, inst, instTabRules)

	// Players.
	b.WriteString(box(0.3, 2.15, 5.1, 2.5, colorCard))
	b.WriteString(sectionTitle(0.55, 2.45, "Players"))
	settingRow(&b, 0.55, 3.0, iconHeart, "setting_damage", "Can get hurt",
		"Players lose health from falls, lava, mobs... Off = nobody can get hurt.", settings.EnableDamage)
	settingRow(&b, 0.55, 3.55, iconPvP, "setting_pvp", "Can hit each other (PvP)",
		"Players can damage other players. Needs 'Can get hurt'.", settings.EnablePVP)
	settingRow(&b, 0.55, 4.1, "", "setting_hunger", "Hunger",
		"Players need to eat. Needs 'Can get hurt'. Applies after a restart.", settings.EnableHunger)

	// World.
	b.WriteString(box(5.6, 2.15, 5.1, 2.5, colorCard))
	b.WriteString(sectionTitle(5.85, 2.45, "World"))
	settingRow(&b, 5.85, 3.0, "", "setting_mobs", "Animals & mobs spawn",
		"Creatures spawn naturally. Applies after a restart.", settings.MobsSpawn)
	settingRow(&b, 5.85, 3.55, "", "setting_peaceful", "Only peaceful mobs",
		"No hostile monsters (zombies, creepers...). Applies after a restart.", settings.OnlyPeacefulMobs)
	settingRow(&b, 5.85, 4.1, "", "setting_explosions", "Explosions break blocks",
		"TNT and creepers destroy builds. Applies after a restart.", settings.ExplosionsGriefing)

	// Spawn point.
	b.WriteString(box(0.3, 4.85, 10.4, 1.5, colorCard))
	b.WriteString(sectionTitle(0.55, 5.15, "Arrival point"))
	spawn := "Not set (default spawn)"
	if settings.StaticSpawnpoint.Valid && strings.TrimSpace(settings.StaticSpawnpoint.String) != "" {
		spawn = settings.StaticSpawnpoint.String
		if settings.SpawnYaw.Valid {
			spawn += fmt.Sprintf("  facing %.0f°", math.Mod(settings.SpawnYaw.Float64*180/math.Pi+360, 360))
		}
	}
	b.WriteString(coloredLbl(0.55, 5.65, light, spawn))
	b.WriteString(hint(0.55, 6.05, "Where players appear when they join. Applies after a restart."))
	if cc.ServerName() == inst.ProxyName {
		b.WriteString(styledBtn(7.3, 5.3, 3.2, 0.65, "btn_capture_spawn", "Use my position", colorPrimary))
		b.WriteString(tooltip("btn_capture_spawn", "Saves where you stand and the direction you look"))
	} else {
		b.WriteString(hint(7.0, 5.5, "Join the world to set it."))
	}

	// Apply / restart banner.
	if c.isRestartPending(inst.ID) {
		b.WriteString(box(0.3, 6.55, 10.4, 1.5, "#4a3a12"))
		b.WriteString(fmt.Sprintf("image[0.55,6.75;0.5,0.5;%s]", iconWarning))
		b.WriteString(coloredLbl(1.25, 7.0, warning, "Some changes apply after a restart."))
		b.WriteString(hint(1.25, 7.5, "Players are brought back automatically."))
		if inst.Status == "running" {
			b.WriteString(styledBtn(7.6, 6.95, 2.9, 0.7, "btn_restart_instance", "Restart now", colorActive))
		} else {
			b.WriteString(hint(7.0, 7.0, "Start the world to apply."))
		}
	} else {
		b.WriteString(box(0.3, 6.55, 10.4, 1.5, colorCard))
		b.WriteString(fmt.Sprintf("image[0.55,6.85;0.42,0.42;%s]", iconCheck))
		b.WriteString(hint(1.25, 7.05, "Changes are saved as soon as you click."))
		b.WriteString(hint(1.25, 7.5, "Hurt and PvP apply at once; the rest after a restart."))
	}

	b.WriteString(btn(0.3, 8.35, 4.5, 0.7, "btn_reset_settings", "Reset to classroom-safe"))
	b.WriteString(tooltip("btn_reset_settings", "No damage, PvP or hunger; peaceful mobs only; explosions keep blocks"))

	cc.ShowFormspec("classrooms:instance_settings", b.String())
}

func (c *controller) showInstanceRestartConfirm(cc *proxy.ClientConn, instanceID string) {
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil || !c.canManageInstance(inst, cc.Name()) {
		c.showInstanceFallback(cc, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	c.setActiveInstanceWithOrigin(cc.Name(), instanceID, c.getActiveInstanceOrigin(cc.Name()))
	players := c.playersOnInstance(inst.ProxyName)

	var b strings.Builder
	dialogFrame(&b, 9, 6.4, warning, iconWarning, "Restart "+inst.Title()+"?")
	b.WriteString(box(0.5, 1.6, 8, 3.3, colorCard))
	b.WriteString(coloredLbl(0.85, 2.1, light, "1. Everyone inside goes to the lobby."))
	b.WriteString(coloredLbl(0.85, 2.6, light, "2. The world restarts with the new rules."))
	b.WriteString(coloredLbl(0.85, 3.1, light, "3. Everyone is brought back automatically."))
	b.WriteString(hint(0.85, 3.7, fmt.Sprintf("Players currently inside: %d", len(players))))
	if len(players) > 0 {
		list := strings.Join(players, ", ")
		if len(list) > 80 {
			list = list[:80] + "…"
		}
		b.WriteString(hint(0.85, 4.2, list))
	}
	b.WriteString(styledBtn(1.0, 5.3, 3.4, 0.75, "btn_confirm_restart", "Restart now", colorActive))
	b.WriteString(btn(5.0, 5.3, 2.6, 0.75, "btn_back", "Cancel"))
	cc.ShowFormspec("classrooms:instance_restart", b.String())
}

func (c *controller) showInstanceFallback(cc *proxy.ClientConn, origin string) {
	switch origin {
	case viewOriginAdminClasses:
		if classID, ok := c.getActiveClass(cc.Name()); ok {
			c.showClassViewWithOrigin(cc, classID, viewOriginAdminClasses)
			return
		}
		c.showAdminPanelTab(cc, "classes")
	case viewOriginAdminInstances:
		c.showAdminPanelTab(cc, "instances")
	default:
		c.showMainDashboard(cc)
	}
}

// ── Admin Panel ─────────────────────────────────────────────────────────────

func (c *controller) showAdminPanel(cc *proxy.ClientConn) {
	c.showAdminPanelTab(cc, "instances")
}

func (c *controller) showAdminPanelTab(cc *proxy.ClientConn, activeTab string) {
	if !c.isAdmin(cc.Name()) {
		c.showMainDashboard(cc)
		return
	}

	if activeTab != "teachers" && activeTab != "classes" {
		activeTab = "instances"
	}
	c.setAdminTab(cc.Name(), activeTab)

	var b strings.Builder
	fsOpen(&b, 14, 8.6)
	fsHeader(&b, 14, "Global admin", "All worlds, classes and teachers", false, true)
	tabBar(&b, 0.3, 1.2, 2.4, "btn_admin_tab_"+activeTab, [][2]string{
		{"btn_admin_tab_instances", "Worlds"},
		{"btn_admin_tab_classes", "Classes"},
		{"btn_admin_tab_teachers", "Teachers"},
	})

	if activeTab == "teachers" {
		c.writeAdminTeachersTab(&b)
	} else if activeTab == "classes" {
		c.writeAdminClassesTab(cc, &b)
	} else {
		c.writeAdminInstancesTab(cc, &b)
	}

	cc.ShowFormspec("classrooms:admin", b.String())
}

func (c *controller) writeAdminInstancesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institute, teacher := c.getAdminFilters(cc.Name())
	instances, _ := c.getFilteredActiveInstances(institute, teacher)

	c.writeAdminFilters(b, institute, teacher)
	if len(instances) == 0 {
		b.WriteString(hint(0.5, 3.35, "No worlds match the current filters."))
		return
	}
	b.WriteString(adminScroll("scr_admin_inst", len(instances)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_inst;vertical;0.1]")
	iy := 0.05
	for _, inst := range instances {
		b.WriteString(box(0, iy, 13.0, 0.85, colorCard))
		b.WriteString(statusDot(0.2, iy+0.18, instanceStatusColor(inst.Status)))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.28, fmtEsc(mcColorize(light, inst.Title()))))
		detail := inst.CreatedBy + " · " + instanceStatusLabel(inst.Status)
		if inst.Institute != "" {
			detail = inst.Institute + " · " + detail
		}
		if inst.DisplayName != "" {
			detail = detail + " · " + inst.ID
		}
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.6, fmtEsc(mcColorize(muted, detail))))
		b.WriteString(iconBtn(12.2, iy+0.13, 0.6, "open_inst_"+inst.ID, iconGear, "Manage world"))
		iy += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

func (c *controller) writeAdminClassesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institute, teacher := c.getAdminFilters(cc.Name())
	classes, _ := c.getFilteredClasses(institute, teacher)

	c.writeAdminFilters(b, institute, teacher)
	if len(classes) == 0 {
		b.WriteString(hint(0.5, 3.35, "No classes match the current filters."))
		return
	}

	b.WriteString(adminScroll("scr_admin_classes", len(classes)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_classes;vertical;0.1]")
	y := 0.05
	for _, cls := range classes {
		students, _ := c.getStudents(cls.ID)
		instances, _ := c.getInstancesForClass(cls.ID)
		b.WriteString(box(0, y, 13.0, 0.85, colorCard))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.28, fmtEsc(mcColorize(light, cls.Name))))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.6, fmtEsc(mcColorize(muted,
			fmt.Sprintf("Owner: %s · %s · %s", cls.CreatedBy, plural(len(students), "student", "students"),
				plural(len(instances), "world", "worlds"))))))
		b.WriteString(styledBtn(10.35, y+0.13, 1.6, 0.6, fmt.Sprintf("open_class_%d", cls.ID), "Open", colorPrimary))
		delName := fmt.Sprintf("del_class_%d", cls.ID)
		if c.isDeleteArmed(cc.Name(), fmt.Sprintf("class:%d", cls.ID)) {
			b.WriteString(styledBtn(12.05, y+0.13, 0.85, 0.6, delName, "Sure?", colorDanger))
		} else {
			b.WriteString(iconBtn(12.2, y+0.13, 0.6, delName, iconClose, "Delete class"))
		}
		y += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

func adminScroll(name string, itemCount int) string {
	return scrollbarFor(name, 13.45, 3.0, 5.25, itemCount, 0.95, 0.05)
}

func scrollbarFor(name string, x, y, h float64, itemCount int, rowStep, topPad float64) string {
	return scrollbarAt(name, x, y, h, itemCount, rowStep, topPad, 0)
}

// scrollbarAt is scrollbarFor starting at a remembered position.
func scrollbarAt(name string, x, y, h float64, itemCount int, rowStep, topPad float64, value int) string {
	const (
		factor = 0.1
	)
	contentHeight := topPad + float64(itemCount)*rowStep
	if contentHeight <= h {
		return ""
	}
	max := int((contentHeight - h) / factor)
	if max < 1 {
		max = 1
	}
	if value > max {
		value = max
	}
	if value < 0 {
		value = 0
	}
	return fmt.Sprintf("scrollbaroptions[min=0;max=%d;smallstep=4;largestep=16;arrows=default]scrollbar[%g,%g;0.25,%g;vertical;%s;%d]", max, x, y, h, name, value)
}

func (c *controller) writeAdminFilters(b *strings.Builder, institute, teacher string) {
	b.WriteString(box(0.2, 2.05, 13.55, 0.8, colorCard))
	b.WriteString(fmt.Sprintf("image[0.4,2.22;0.45,0.45;%s]", iconSearch))
	b.WriteString(hint(1.0, 2.45, "Institute"))
	b.WriteString("field[2.1,2.2;3.1,0.5;admin_filter_institute;;")
	b.WriteString(fmtEsc(institute))
	b.WriteString("]")
	b.WriteString(hint(5.45, 2.45, "Teacher"))
	b.WriteString("field[6.4,2.2;3.1,0.5;admin_filter_teacher;;")
	b.WriteString(fmtEsc(teacher))
	b.WriteString("]")
	b.WriteString(styledBtn(9.8, 2.2, 1.6, 0.5, "btn_admin_filter_apply", "Filter", colorPrimary))
	b.WriteString(btn(11.55, 2.2, 1.6, 0.5, "btn_admin_filter_clear", "Clear"))
}

func (c *controller) writeAdminTeachersTab(b *strings.Builder) {
	teachers, _ := c.listTeacherRecords()

	b.WriteString(box(0.2, 2.05, 13.55, 0.8, colorCard))
	b.WriteString(fmt.Sprintf("image[0.4,2.22;0.45,0.45;%s]", iconPlus))
	b.WriteString(hint(1.0, 2.45, "Username"))
	b.WriteString("field[2.2,2.2;3.1,0.5;new_teacher_name;;]")
	b.WriteString(hint(5.45, 2.45, "Institute"))
	b.WriteString("field[6.4,2.2;3.1,0.5;new_teacher_institute;;]")
	b.WriteString(styledBtn(9.8, 2.2, 1.6, 0.5, "btn_admin_add_teacher", "Add", colorPrimary))

	b.WriteString(adminScroll("scr_admin_teachers", len(teachers)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_teachers;vertical;0.1]")
	ty := 0.05
	for _, t := range teachers {
		fieldName := "teacher_institute_" + t.Username
		b.WriteString(box(0, ty, 13.0, 0.85, colorCard))
		b.WriteString(fmt.Sprintf("image[0.2,%g;0.45,0.45;%s]", ty+0.2, iconPlayer))
		b.WriteString(fmt.Sprintf("label[0.8,%g;%s]", ty+0.42, fmtEsc(t.Username)))
		b.WriteString(fmt.Sprintf("field[4.1,%g;5.0,0.5;%s;;%s]", ty+0.17, fmtEsc(fieldName), fmtEsc(t.Institute)))
		b.WriteString(fmt.Sprintf("tooltip[%s;Institute]", fmtEsc(fieldName)))
		b.WriteString(btn(9.35, ty+0.14, 1.4, 0.58, "save_teacher_"+t.Username, "Save"))
		b.WriteString(iconBtn(12.2, ty+0.13, 0.6, "rm_teacher_"+t.Username, iconClose, "Remove teacher role"))
		ty += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

// ── People editor: Students | Assistance | Teachers ─────────────────────────

const (
	peopleTabStudents   = "tab_students"
	peopleTabAssistance = "tab_assistance"
	peopleTabTeachers   = "tab_teachers"
)

func (c *controller) peopleFrame(b *strings.Builder, cc *proxy.ClientConn, cls *classData, activeTab string) {
	c.peopleFrameWidth(b, cc, cls, activeTab, 9)
}

func (c *controller) peopleFrameWidth(b *strings.Builder, cc *proxy.ClientConn, cls *classData, activeTab string, w float64) {
	fsOpen(b, w, 9.6)
	fsHeader(b, w, "People", cls.Name, true, true)
	tabs := [][2]string{{peopleTabStudents, "Students"}}
	if c.canManageClass(cls.ID, cc.Name()) {
		tabs = append(tabs, [2]string{peopleTabGroups, "Groups"},
			[2]string{peopleTabAssistance, "Assistance"}, [2]string{peopleTabTeachers, "Teachers"})
	}
	tabBar(b, 0.3, 1.2, 1.95, activeTab, tabs)
}

// personList draws a scrollable list of names with a remove button each.
func personList(b *strings.Builder, y, h float64, names []string, removePrefix, removeTip string) {
	if len(names) == 0 {
		b.WriteString(hint(0.6, y+0.4, "Nobody yet."))
		return
	}
	b.WriteString(scrollbarFor("scr_people", 8.4, y, h, len(names), 0.7, 0.05))
	b.WriteString(fmt.Sprintf("scroll_container[0.3,%g;8.0,%g;scr_people;vertical;0.1]", y, h))
	ry := 0.05
	for _, n := range names {
		b.WriteString(box(0, ry, 7.95, 0.62, colorRow))
		dot := muted
		if proxy.Find(n) != nil {
			dot = success
		}
		b.WriteString(statusDot(0.2, ry+0.2, dot))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", ry+0.31, fmtEsc(n)))
		b.WriteString(iconBtn(7.3, ry+0.06, 0.5, removePrefix+n, iconClose, removeTip+" "+n))
		ry += 0.7
	}
	b.WriteString("scroll_container_end[]")
}

func (c *controller) showStudentEditor(cc *proxy.ClientConn, classID int) {
	cls, _ := c.getClassByID(classID)
	if cls == nil || !c.canEditClassStudents(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	students, _ := c.getStudents(classID)

	var b strings.Builder
	c.peopleFrame(&b, cc, cls, peopleTabStudents)

	b.WriteString(box(0.3, 2.05, 8.4, 1.25, colorCard))
	b.WriteString(sectionTitle(0.55, 2.3, "Add a player who already has an account"))
	b.WriteString("field[0.55,2.55;5.6,0.55;add_student_name;;]")
	b.WriteString("field_close_on_enter[add_student_name;false]")
	b.WriteString(styledBtn(6.35, 2.55, 2.1, 0.55, "btn_add_student", "+ Add", colorPrimary))

	listY, listHeight := 3.85, 5.55
	if c.canManageClass(classID, cc.Name()) {
		b.WriteString(box(0.3, 3.45, 8.4, 2.65, colorCard))
		b.WriteString(sectionTitle(0.55, 3.7, "Or create a new account"))
		b.WriteString("field[0.55,4.4;3.9,0.55;new_student_name;Username;]")
		b.WriteString("pwdfield[0.55,5.35;3.9,0.55;new_student_password;Password (8+ characters)]")
		b.WriteString("pwdfield[4.6,5.35;3.85,0.55;new_student_confirm;Repeat password]")
		b.WriteString(styledBtn(4.6, 4.4, 3.85, 0.55, "btn_create_student", "Create and add", colorPrimary))
		for _, field := range []string{"new_student_name", "new_student_password", "new_student_confirm"} {
			b.WriteString("field_close_on_enter[" + field + ";false]")
		}
		listY, listHeight = 6.6, 2.85
	}
	b.WriteString(sectionTitle(0.45, listY-0.2, fmt.Sprintf("In this class (%d)", len(students))))
	personList(&b, listY, listHeight-0.1, students, "rm_student_", "Remove from class:")

	cc.ShowFormspec("classrooms:students", b.String())
}

func (c *controller) showClassMemberEditor(cc *proxy.ClientConn, classID int, assistance bool) {
	cls, _ := c.getClassByID(classID)
	if cls == nil || !c.canManageClass(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	members, _ := c.getClassTeachers(classID)
	tab, formName, fieldName, addButton, removePrefix := peopleTabTeachers, "classrooms:teachers", "add_teacher_name", "btn_add_teacher", "rm_teacher_"
	explain := "Co-teachers have the same controls as you."
	if assistance {
		members, _ = c.getClassAssistants(classID)
		tab, formName, fieldName, addButton, removePrefix = peopleTabAssistance, "classrooms:assistants", "add_assistance_name", "btn_add_assistance", "rm_assistance_"
		explain = "Assistance can add students and join class worlds."
	}

	var b strings.Builder
	c.peopleFrame(&b, cc, cls, tab)
	b.WriteString(box(0.3, 2.05, 8.4, 1.6, colorCard))
	b.WriteString(hint(0.55, 2.35, explain))
	b.WriteString(fmt.Sprintf("field[0.55,2.85;5.6,0.55;%s;;]", fieldName))
	b.WriteString(fmt.Sprintf("field_close_on_enter[%s;false]", fieldName))
	b.WriteString(styledBtn(6.35, 2.85, 2.1, 0.55, addButton, "+ Add", colorPrimary))
	listTop := 4.25
	if !assistance {
		b.WriteString(hint(0.45, 3.95, "Owner: "+cls.CreatedBy))
		listTop = 4.45
	} else {
		b.WriteString(sectionTitle(0.45, 3.95, fmt.Sprintf("Assistance (%d)", len(members))))
	}
	personList(&b, listTop, 9.4-listTop, members, removePrefix, "Remove")
	cc.ShowFormspec(formName, b.String())
}
