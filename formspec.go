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

var classViewFixedFilters = [][2]string{{"all", T("All students")}, {"online", T("Online now")}, {"none", T("Without group")}}

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
		cc.SendChatMsg("[Classrooms] " + T("Error loading classes: @1", err.Error()))
		return
	}

	var b strings.Builder
	fsOpen(&b, 12, 9)
	subtitle := T("Assistance dashboard")
	if isFullTeacher {
		subtitle = T("Teacher dashboard")
	}
	fsHeader(&b, 12, T("Classrooms"), subtitle, false, true)
	if c.isAdmin(name) {
		b.WriteString(iconBtn(10.35, 0.17, 0.66, "btn_admin_panel", iconGear, T("Admin panel: all worlds, classes and teachers")))
	}

	listY := 2.0
	if isFullTeacher {
		b.WriteString(box(0.3, 1.3, 11.4, 0.95, colorCard))
		if limitMsg := c.classLimitMessage(name); limitMsg != "" && !c.isAdmin(name) {
			b.WriteString(fmt.Sprintf("image[0.55,1.53;0.45,0.45;%s]", iconWarning))
			b.WriteString(coloredLbl(1.15, 1.78, warning, T("Class limit reached")))
			b.WriteString(disabledBtn(9.0, 1.48, 2.5, 0.6, T("Create class"), limitMsg))
		} else {
			b.WriteString(hint(0.55, 1.78, T("New class")))
			b.WriteString("field[1.9,1.48;6.9,0.6;new_class_name;;]")
			b.WriteString("field_close_on_enter[new_class_name;false]")
			b.WriteString(tooltip("new_class_name", T("Type a name for the new class, e.g. 3B Science")))
			b.WriteString(styledBtn(9.0, 1.48, 2.5, 0.6, "btn_create_class", T("Create class"), colorPrimary))
		}
		listY = 2.75
	}
	b.WriteString(sectionTitle(0.35, listY, T("YOUR CLASSES")))
	if isFullTeacher {
		if in, used, ok := c.instituteClassUsage(name); ok {
			usage := T("@1 · classes: @2 (no limit)", in.Name, used)
			color := muted
			if in.MaxClasses > 0 {
				usage = T("@1 · classes: @2 of @3", in.Name, used, in.MaxClasses)
				if used >= in.MaxClasses {
					color = warning
				}
			}
			b.WriteString(fmt.Sprintf("label[4.0,%g;%s]", listY, fmtEsc(mcColorize(color, usage))))
		}
	}

	if len(classes) == 0 {
		emptyText := T("No classes are assigned to you yet.")
		if isFullTeacher {
			emptyText = T("No classes yet: type a name above and press Create class.")
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
			T("@1/@2 students online   ·   @3 worlds running", online, len(students), worldsOnline)))))

		b.WriteString(styledBtn(7.55, y+0.22, 1.9, 0.62, fmt.Sprintf("open_class_%d", cls.ID), T("Open"), colorPrimary))
		if cls.CreatedBy == name || c.isAdmin(name) {
			delName := fmt.Sprintf("del_class_%d", cls.ID)
			if c.isDeleteArmed(name, fmt.Sprintf("class:%d", cls.ID)) {
				b.WriteString(styledBtn(9.55, y+0.22, 1.3, 0.62, delName, T("Delete?"), colorDanger))
				b.WriteString(tooltip(delName, T("Click again to delete this class permanently")))
			} else {
				b.WriteString(iconBtn(10.2, y+0.22, 0.62, delName, iconClose, T("Delete class")))
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
	subtitle := T("@1/@2 students online", online, len(students))
	if origin == viewOriginAdminClasses {
		subtitle += "   ·   " + T("Owner: @1", cls.CreatedBy)
	}
	fsHeader(&b, 16, cls.Name, subtitle, true, true)
	if canManage {
		// Same place and color as "Return to HUB" in the students' world map.
		b.WriteString(styledBtn(12.3, 0.2, 2.65, 0.6, "btn_return_hub", T("Return to HUB"), colorDanger))
		b.WriteString(tooltip("btn_return_hub", T("Go back to the HUB, alone or with the students in the class worlds")))
		b.WriteString(styledBtn(9.5, 0.2, 2.65, 0.6, "btn_minigames", T("Minigames"), colorPrimary))
		b.WriteString(tooltip("btn_minigames", T("Start a HUB minigame for the class or one of its groups")))
	}

	// Left column: live controls and students.
	b.WriteString(box(0.2, 1.3, 8.6, 8.1, colorCard))
	listY := 2.35
	if canManage {
		b.WriteString(sectionTitle(0.45, 1.6, T("LIVE CONTROLS (ONLINE STUDENTS)")))
		frozen := c.isClassFrozen(classID)
		watching := c.isClassWatching(classID, cc.Name())
		freezeCaption := T("Freeze")
		if frozen {
			freezeCaption = T("Unfreeze")
		}
		watchCaption := T("Look at me")
		if watching {
			watchCaption = T("Stop looking")
		}
		actionTile(&b, 0.45, 1.95, 2.6, "btn_toggle_freeze", iconFreeze, freezeCaption,
			T("Stops every online student from moving. Click again to release them."), frozen)
		actionTile(&b, 3.2, 1.95, 2.6, "btn_gather_all", iconGather, T("Bring here"),
			T("Teleports every online student around you."), false)
		actionTile(&b, 5.95, 1.95, 2.6, "btn_toggle_watch", iconEye, watchCaption,
			T("Turns every online student's camera towards you."), watching)
		listY = 4.0
	}

	b.WriteString(sectionTitle(0.45, listY, T("STUDENTS")))
	b.WriteString(styledBtn(5.55, listY-0.3, 3.0, 0.58, "btn_people", T("Manage people"), colorButton))
	b.WriteString(tooltip("btn_people", T("Add or remove students, create student accounts, and manage groups, assistance and teachers")))
	groups, _ := c.getGroups(classID)
	byStudent := groupByStudent(groups)
	view := c.studentList(cc.Name(), classID)
	filterLabels, filterKeys := filterOptions(groups, classViewFixedFilters)
	b.WriteString(fmt.Sprintf("dropdown[2.1,%g;3.3,0.58;cls_filter;%s;%d;true]",
		listY-0.3, dropdownItems(filterLabels), indexOf(filterKeys, view.ClassFilter)))
	b.WriteString(tooltip("cls_filter", T("Show only some students")))
	shown := onlineFirst(filterStudents(students, byStudent, view.ClassFilter, ""))
	listTop := listY + 0.4
	listH := 9.25 - listTop
	if len(students) == 0 {
		b.WriteString(hint(0.6, listTop+0.4, T("No students yet: use Manage people to add them.")))
	} else if len(shown) == 0 {
		b.WriteString(hint(0.6, listTop+0.4, T("No students match this filter.")))
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
			b.WriteString(iconBtn(6.55, sy+0.06, 0.5, "tp_to_"+s, iconTeleport, T("Teleport to @1", s)))
			if canManage {
				b.WriteString(iconBtn(7.2, sy+0.06, 0.5, "watch_"+s, iconEye, T("Make @1 look at you", s)))
			}
		} else {
			b.WriteString(fmt.Sprintf("label[6.55,%g;%s]", sy+0.31, fmtEsc(mcColorize(muted, T("offline")))))
		}
		sy += 0.7
	}
	b.WriteString("scroll_container_end[]")

	// Right column: class worlds.
	b.WriteString(box(9.0, 1.3, 6.8, 8.1, colorCard))
	b.WriteString(sectionTitle(9.25, 1.6, T("CLASS WORLDS")))
	maxWorlds := c.classWorldLimit(cls)
	worldsFull := maxWorlds > 0 && len(instances) >= maxWorlds
	if maxWorlds > 0 {
		color := muted
		if worldsFull {
			color = warning
		}
		b.WriteString(fmt.Sprintf("label[12.2,1.6;%s]", fmtEsc(mcColorize(color, T("@1 of @2 worlds", len(instances), maxWorlds)))))
	}
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
		if worldsFull && !c.isAdmin(cc.Name()) {
			b.WriteString(disabledBtn(9.25, worldTop-0.05, 6.3, 0.65, T("World limit reached"), c.worldLimitMessage(classID)))
		} else {
			b.WriteString(styledBtn(9.25, worldTop-0.05, 6.3, 0.65, "btn_create_instance", T("+ New class world"), colorPrimary))
			b.WriteString(tooltip("btn_create_instance", T("Create a new world from a map template for this class")))
		}
		worldTop += 0.8
	}
	worldBottom := 9.25
	if len(worlds) == 0 {
		msg := T("No class worlds are running right now.")
		if canManage {
			msg = T("No worlds yet: press + New class world.")
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
				mcColorize(muted, "  ·  "+c.templateLabel(inst.TemplateName)))))
		bx := 0.2
		if inst.Status == "running" {
			here := cc.ServerName() == inst.ProxyName
			if !here {
				b.WriteString(styledBtn(bx, iy+0.93, 1.5, 0.42, "join_inst_"+inst.ID, T("Join"), colorPrimary))
				b.WriteString(tooltip("join_inst_"+inst.ID, T("Go to this world")))
				bx += 1.6
			} else {
				b.WriteString(fmt.Sprintf("label[%g,%g;%s]", bx, iy+1.14, fmtEsc(mcColorize(success, T("You are here")))))
				bx += 2.1
			}
			if canManage {
				b.WriteString(btn(bx, iy+0.93, 2.0, 0.42, "bring_inst_"+inst.ID, T("Bring class")))
				b.WriteString(tooltip("bring_inst_"+inst.ID, T("Move you and every online student of this class into this world")))
			}
		}
		if canManage {
			b.WriteString(iconBtn(5.45, iy+0.15, 0.55, "open_inst_"+inst.ID, iconGear, T("Manage: start/stop, rules, time & weather")))
		}
		iy += 1.55
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:class", b.String())
}

// showReturnHubDialog asks whether the students in the class worlds come
// back to the HUB with the teacher.
func (c *controller) showReturnHubDialog(cc *proxy.ClientConn, classID int) {
	n := len(c.studentsInClassWorlds(classID))
	var b strings.Builder
	dialogFrame(&b, 9, 5.6, accent, iconEnter, T("Return to HUB"))
	b.WriteString(box(0.5, 1.6, 8, 1.5, colorCard))
	b.WriteString(coloredLbl(0.85, 2.05, light, T("@1 students of this class are in its worlds.", n)))
	b.WriteString(hint(0.85, 2.6, T("Do they come back to the HUB with you?")))
	b.WriteString(styledBtn(0.5, 3.4, 3.9, 0.75, "btn_hub_with_students", T("Me and the students"), colorPrimary))
	b.WriteString(btn(4.6, 3.4, 3.9, 0.75, "btn_hub_alone", T("Only me")))
	b.WriteString(btn(3.0, 4.55, 3.0, 0.6, "btn_hub_cancel", T("Cancel")))
	cc.ShowFormspec("classrooms:return_hub", b.String())
}

// onOff renders a boolean rule for the current-world summary.
func onOff(v bool) string {
	if v {
		return T("on")
	}
	return T("off")
}

// writeCurrentWorldCard highlights the class world the viewer is standing in,
// with a summary of its rules and shortcuts to change them.
func (c *controller) writeCurrentWorldCard(b *strings.Builder, inst *instanceData, canManage bool) {
	b.WriteString(box(9.2, 1.95, 6.4, 2.5, colorCurrent))
	b.WriteString(box(9.2, 1.95, 0.08, 2.5, colorActive))
	b.WriteString(coloredLbl(9.45, 2.22, warning, T("YOU ARE IN THIS WORLD")))
	if c.isRestartPending(inst.ID) {
		b.WriteString(fmt.Sprintf("image[13.2,2.04;0.36,0.36;%s]", iconWarning))
		b.WriteString(coloredLbl(13.65, 2.22, warning, T("restart needed")))
	}
	b.WriteString(coloredLbl(9.45, 2.62, light, inst.Title()))

	if settings, err := c.getInstanceSettingsOrDefault(inst.ID); err == nil {
		b.WriteString(hint(9.45, 3.02, T("Hurt: @1  ·  PvP: @2  ·  Hunger: @3",
			onOff(settings.EnableDamage), onOff(settings.EnablePVP), onOff(settings.EnableHunger))))
		mobs := T("no mobs")
		if settings.MobsSpawn && settings.OnlyPeacefulMobs {
			mobs = T("peaceful mobs")
		} else if settings.MobsSpawn {
			mobs = T("all mobs")
		}
		timeState := T("time flowing")
		if c.isInstanceTimeStopped(inst.ID) {
			timeState = T("time paused")
		}
		if settings.WorldLocked {
			timeState += "  ·  " + T("world locked")
		}
		b.WriteString(hint(9.45, 3.38, T("Mobs: @1", mobs)+"  ·  "+timeState))
	}

	if canManage {
		b.WriteString(styledBtn(9.45, 3.75, 2.4, 0.52, "cur_rules_"+inst.ID, T("Rules"), colorButton))
		b.WriteString(tooltip("cur_rules_"+inst.ID, T("Damage, PvP, hunger, mobs and arrival point of this world")))
		b.WriteString(styledBtn(11.95, 3.75, 2.85, 0.52, "cur_world_"+inst.ID, T("Time & weather"), colorButton))
		b.WriteString(iconBtn(14.95, 3.75, 0.52, "open_inst_"+inst.ID, iconGear, T("Manage this world")))
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
	fsHeader(&b, 11, T("New class world"), T("Name it, then pick a map"), true, true)

	b.WriteString(box(0.3, 1.3, 10.4, 1.15, colorCard))
	b.WriteString(coloredLbl(0.55, 1.6, warning, "1"))
	b.WriteString(coloredLbl(0.85, 1.6, light, T("World name")))
	b.WriteString("field[0.55,1.85;9.9,0.5;new_instance_name;;]")
	b.WriteString("field_close_on_enter[new_instance_name;false]")

	b.WriteString(coloredLbl(0.55, 2.85, warning, "2"))
	b.WriteString(coloredLbl(0.85, 2.85, light, T("Choose a map")))
	b.WriteString(scrollbarFor("scr_templates", 10.45, 3.15, 4.55, len(templates), 1.05, 0.05))
	b.WriteString("scroll_container[0.3,3.15;10.1,4.55;scr_templates;vertical;0.1]")
	y := 0.05
	for _, tName := range templates {
		tpl := c.cfg.Templates[tName]
		b.WriteString(box(0, y, 10.0, 0.95, colorCard))
		b.WriteString(fmt.Sprintf("image[0.15,%g;0.75,0.75;%s]", y+0.1, c.templateIcon(tName)))
		b.WriteString(fmt.Sprintf("label[1.1,%g;%s]", y+0.3, fmtEsc(mcColorize(light, c.templateLabel(tName)))))
		b.WriteString(fmt.Sprintf("label[1.1,%g;%s]", y+0.66, fmtEsc(mcColorize(muted, T(tpl.ServerDescription)))))
		b.WriteString(styledBtn(8.0, y+0.17, 1.8, 0.6, "pick_tpl_"+tName, T("Create"), colorPrimary))
		y += 1.05
	}
	b.WriteString("scroll_container_end[]")

	b.WriteString(fmt.Sprintf("image[0.35,7.9;0.4,0.4;%s]", iconCheck))
	b.WriteString(hint(0.9, 8.1, T("New worlds start safe: no damage or hunger, peaceful mobs.")))

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
	b.WriteString(hint(0.85, 2.85, T("This can take a minute. You can close this window:")))
	b.WriteString(hint(0.85, 3.35, T("a new one opens when the world is ready.")))
	b.WriteString(btnExit(3.0, 4.4, 3.0, 0.7, "btn_progress_close", T("Close")))
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
	b.WriteString(hint(0.85, 2.7, T("Map: @1   ·   The world is online. What next?", c.templateLabel(inst.TemplateName))))

	tileW := 2.7
	x := 0.5
	actionTile(&b, x, 3.7, tileW, "btn_ready_hop_me", iconEnter, T("Go there"), T("Join the world yourself"), false)
	x += tileW + 0.45
	if inst.ClassID != nil {
		actionTile(&b, x, 3.7, tileW, "btn_ready_hop_class", iconPeople, T("Bring class"), T("Move you and every online student into the world"), false)
		x += tileW + 0.45
	}
	actionTile(&b, x, 3.7, tileW, "btn_ready_open", iconGear, T("Manage"), T("Rules, time & weather, start/stop"), false)
	b.WriteString(btnExit(3.7, 5.85, 2.6, 0.55, "btn_ready_close", T("Close")))

	cc.ShowFormspec("classrooms:instance_ready", b.String())
}

func (c *controller) showInstanceError(cc *proxy.ClientConn, inst *instanceData, title, detail string) {
	if inst != nil {
		c.setActiveInstance(cc.Name(), inst.ID)
	}

	var b strings.Builder
	dialogFrame(&b, 10, 6.6, danger, iconError, title)
	b.WriteString(box(0.5, 1.6, 9, 3.5, colorCard))
	b.WriteString(coloredLbl(0.85, 2.05, light, T("The operation did not complete. Details:")))
	b.WriteString(fmt.Sprintf("textarea[0.85,2.4;8.3,2.5;error_detail;;%s]", fmtEsc(detail)))

	if inst != nil {
		b.WriteString(btn(1.0, 5.5, 2.6, 0.75, "btn_error_open", T("Manage world")))
	}
	b.WriteString(btnExit(6.4, 5.5, 2.6, 0.75, "btn_error_close", T("Close")))

	cc.ShowFormspec("classrooms:instance_error", b.String())
}

// ── World (Instance) View: Overview | Rules | Time & weather ────────────────

const (
	instTabOverview = "tab_overview"
	instTabRules    = "tab_rules"
	instTabWorld    = "tab_world"
)

// instanceFrame draws the shared header and tab bar of the world screens.
func (c *controller) instanceFrame(b *strings.Builder, inst *instanceData, activeTab string) {
	fsOpen(b, 11, 9.6)
	subtitle := instanceStatusLabel(inst.Status) + "   ·   " + c.templateLabel(inst.TemplateName)
	fsHeader(b, 11, inst.Title(), subtitle, true, true)
	tabBar(b, 0.3, 1.3, 3.4, activeTab, [][2]string{
		{instTabOverview, T("Overview")},
		{instTabRules, T("Rules")},
		{instTabWorld, T("Time & weather")},
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
	c.instanceFrame(&b, inst, instTabOverview)

	// Main actions.
	b.WriteString(box(0.3, 2.15, 10.4, 2.2, colorCard))
	switch inst.Status {
	case "running":
		here := cc.ServerName() == inst.ProxyName
		joinCaption := T("Go there")
		if here {
			joinCaption = T("You are here")
		}
		actionTile(&b, 0.5, 2.3, 3.2, "btn_hop_me", iconEnter, joinCaption, T("Join this world yourself"), here)
		if inst.ClassID != nil {
			actionTile(&b, 3.9, 2.3, 3.2, "btn_hop_class", iconPeople, T("Bring class"),
				T("Move you and every online student of the class into this world"), false)
		}
		actionTile(&b, 7.3, 2.3, 3.2, "btn_inst_stop", iconClose, T("Stop world"),
			T("Sends everyone inside back to the lobby and turns the world off. Nothing is lost."), false)
	case "stopped":
		actionTile(&b, 0.5, 2.3, 3.2, "btn_inst_start", iconNext, T("Start world"),
			T("Turns the world on. It takes about a minute."), false)
		b.WriteString(hint(4.0, 2.95, T("The world is off. Start it to play;")))
		b.WriteString(hint(4.0, 3.4, T("everything built so far is kept.")))
	default:
		b.WriteString(fmt.Sprintf("image[0.6,2.6;0.6,0.6;%s]", iconRefresh))
		b.WriteString(coloredLbl(1.4, 2.9, warning, T("The world is being created. This takes a minute or two.")))
	}

	// Details.
	b.WriteString(box(0.3, 4.55, 10.4, 1.5, colorCard))
	b.WriteString(sectionTitle(0.55, 4.85, T("DETAILS")))
	b.WriteString(hint(0.55, 5.3, T("Created by: @1", inst.CreatedBy)))
	if inst.Institute != "" {
		b.WriteString(hint(5.4, 5.3, T("Institute: @1", inst.Institute)))
	}
	b.WriteString(hint(0.55, 5.75, T("Join command: /join @1", inst.ProxyName)))

	// Guests.
	b.WriteString(box(0.3, 6.25, 10.4, 1.4, colorCard))
	b.WriteString(sectionTitle(0.55, 6.55, T("INVITE A GUEST")))
	b.WriteString(hint(4.0, 6.55, T("Lets a player outside the class join this world")))
	b.WriteString("field[0.55,6.85;7.4,0.6;invite_name;;]")
	b.WriteString("field_close_on_enter[invite_name;false]")
	b.WriteString(tooltip("invite_name", T("Player name to invite")))
	b.WriteString(styledBtn(8.15, 6.85, 2.35, 0.6, "btn_invite", T("+ Invite"), colorPrimary))

	// Danger zone.
	b.WriteString(box(0.3, 7.85, 10.4, 1.5, colorCard))
	b.WriteString(sectionTitle(0.55, 8.15, T("DELETE")))
	if c.isDeleteArmed(cc.Name(), "instance:"+inst.ID) {
		b.WriteString(coloredLbl(0.55, 8.7, danger, T("Sure? This cannot be undone.")))
		b.WriteString(styledBtn(7.3, 8.35, 3.2, 0.7, "btn_inst_delete", T("Yes, delete forever"), colorDanger))
	} else {
		b.WriteString(hint(0.55, 8.7, T("Removes the world and all builds.")))
		b.WriteString(styledBtn(7.3, 8.35, 3.2, 0.7, "btn_inst_delete", T("Delete world"), colorDanger))
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
		c.showInstanceError(cc, inst, T("Settings error"), err.Error())
		return
	}

	var b strings.Builder
	c.instanceFrame(&b, inst, instTabRules)

	// Players.
	b.WriteString(box(0.3, 2.15, 5.1, 3.15, colorCard))
	b.WriteString(sectionTitle(0.55, 2.45, T("PLAYERS")))
	settingRow(&b, 0.55, 2.95, iconHeart, "setting_damage", T("Can get hurt"),
		T("Players lose health from falls, lava, mobs... Off = nobody can get hurt. Teachers, creative and flying players never get hurt. Applies at once."), settings.EnableDamage)
	settingRow(&b, 0.55, 3.45, iconPvP, "setting_pvp", T("Can hit each other (PvP)"),
		T("Players can damage other players. Needs 'Can get hurt'. Applies at once."), settings.EnablePVP)
	settingRow(&b, 0.55, 3.95, "", "setting_hunger", T("Hunger"),
		T("Players need to eat. Teachers, creative and flying players never get hungry. Applies at once."), settings.EnableHunger)
	settingRow(&b, 0.55, 4.45, "", "setting_student_fly", T("Students can fly"),
		T("Students may fly (K) and move fast (J). Teachers always can. Flying players don't get hurt or hungry. Applies at once."), settings.StudentFly)
	settingRow(&b, 0.55, 4.95, "", "setting_student_creative", T("Student creative mode"),
		T("Students get unlimited blocks and items and break blocks instantly. Applies at once."), settings.StudentCreative)

	// World.
	b.WriteString(box(5.6, 2.15, 5.1, 3.15, colorCard))
	b.WriteString(sectionTitle(5.85, 2.45, T("WORLD")))
	settingRow(&b, 5.85, 2.95, "", "setting_mobs", T("Animals & mobs spawn"),
		T("Creatures spawn naturally. Applies after a restart."), settings.MobsSpawn)
	settingRow(&b, 5.85, 3.45, "", "setting_peaceful", T("Only peaceful mobs"),
		T("No hostile monsters (zombies, creepers...). Applies after a restart."), settings.OnlyPeacefulMobs)
	settingRow(&b, 5.85, 3.95, "", "setting_explosions", T("Explosions break blocks"),
		T("TNT and creepers destroy builds. Applies after a restart."), settings.ExplosionsGriefing)
	settingRow(&b, 5.85, 4.45, iconLock, "setting_locked", T("Lock the world"),
		T("Students can only look around, like spectators: no building, digging, using blocks or hitting. Teachers are not affected. Applies at once."), settings.WorldLocked)

	// Spawn point.
	b.WriteString(box(0.3, 5.45, 10.4, 1.4, colorCard))
	b.WriteString(sectionTitle(0.55, 5.75, T("ARRIVAL POINT")))
	spawn := T("Not set (default spawn)")
	if settings.StaticSpawnpoint.Valid && strings.TrimSpace(settings.StaticSpawnpoint.String) != "" {
		spawn = settings.StaticSpawnpoint.String
		if settings.SpawnYaw.Valid {
			spawn += "  " + T("facing @1°", fmt.Sprintf("%.0f", math.Mod(settings.SpawnYaw.Float64*180/math.Pi+360, 360)))
		}
	}
	b.WriteString(coloredLbl(0.55, 6.2, light, spawn))
	b.WriteString(hint(0.55, 6.55, T("Where players appear on join and respawn: at once.")))
	if cc.ServerName() == inst.ProxyName {
		b.WriteString(styledBtn(7.3, 5.85, 3.2, 0.65, "btn_capture_spawn", T("Use my position"), colorPrimary))
		b.WriteString(tooltip("btn_capture_spawn", T("Saves where you stand and the direction you look")))
	} else {
		b.WriteString(hint(7.0, 6.05, T("Join the world to set it.")))
	}

	// Apply / restart banner.
	if c.isRestartPending(inst.ID) {
		b.WriteString(box(0.3, 7.0, 10.4, 1.35, "#4a3a12"))
		b.WriteString(fmt.Sprintf("image[0.55,7.15;0.5,0.5;%s]", iconWarning))
		b.WriteString(coloredLbl(1.25, 7.4, warning, T("Some changes apply after a restart.")))
		b.WriteString(hint(1.25, 7.9, T("Players are brought back automatically.")))
		if inst.Status == "running" {
			b.WriteString(styledBtn(7.6, 7.32, 2.9, 0.7, "btn_restart_instance", T("Restart now"), colorActive))
		} else {
			b.WriteString(hint(7.0, 7.65, T("Start the world to apply.")))
		}
	} else {
		b.WriteString(box(0.3, 7.0, 10.4, 1.35, colorCard))
		b.WriteString(fmt.Sprintf("image[0.55,7.25;0.42,0.42;%s]", iconCheck))
		b.WriteString(hint(1.25, 7.45, T("Changes are saved as soon as you click.")))
		b.WriteString(hint(1.25, 7.9, T("Only mobs and explosions wait for a restart.")))
	}

	b.WriteString(btn(0.3, 8.6, 4.5, 0.7, "btn_reset_settings", T("Reset to classroom-safe")))
	b.WriteString(tooltip("btn_reset_settings", T("No damage, PvP or hunger; peaceful mobs only; explosions keep blocks; students can't fly, survival mode")))

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
	dialogFrame(&b, 9, 6.4, warning, iconWarning, T("Restart @1?", inst.Title()))
	b.WriteString(box(0.5, 1.6, 8, 3.3, colorCard))
	b.WriteString(coloredLbl(0.85, 2.1, light, T("1. Everyone inside goes to the lobby.")))
	b.WriteString(coloredLbl(0.85, 2.6, light, T("2. The world restarts with the new rules.")))
	b.WriteString(coloredLbl(0.85, 3.1, light, T("3. Everyone is brought back automatically.")))
	b.WriteString(hint(0.85, 3.7, T("Players currently inside: @1", len(players))))
	if len(players) > 0 {
		list := strings.Join(players, ", ")
		if len(list) > 80 {
			list = list[:80] + "…"
		}
		b.WriteString(hint(0.85, 4.2, list))
	}
	b.WriteString(styledBtn(1.0, 5.3, 3.4, 0.75, "btn_confirm_restart", T("Restart now"), colorActive))
	b.WriteString(btn(5.0, 5.3, 2.6, 0.75, "btn_back", T("Cancel")))
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

	switch activeTab {
	case "classes", "institutes", "users":
	default:
		activeTab = "instances"
	}
	c.setAdminTab(cc.Name(), activeTab)

	var b strings.Builder
	w, h := 14.0, 8.6
	if activeTab == "users" {
		w, h = usersPanelW, usersPanelH
	}
	fsOpen(&b, w, h)
	fsHeader(&b, w, T("Global admin"), T("All worlds, classes, institutes and users"), false, true)
	tabBar(&b, 0.3, 1.2, 2.4, "btn_admin_tab_"+activeTab, [][2]string{
		{"btn_admin_tab_instances", T("Worlds")},
		{"btn_admin_tab_classes", T("Classes")},
		{"btn_admin_tab_institutes", T("Institutes")},
		{"btn_admin_tab_users", T("Users")},
	})

	if activeTab == "institutes" {
		c.writeAdminInstitutesTab(cc, &b)
	} else if activeTab == "users" {
		c.writeAdminUsersTab(cc, &b)
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
		b.WriteString(hint(0.5, 3.35, T("No worlds match the current filters.")))
		return
	}
	visits := c.getLastWorldVisits()
	classNames := make(map[int]string)
	if classes, err := c.getAllClasses(); err == nil {
		for _, cls := range classes {
			classNames[cls.ID] = cls.Name
		}
	}

	const rowH, rowStep = 1.15, 1.25
	b.WriteString(scrollbarFor("scr_admin_inst", 13.45, 3.0, 5.25, len(instances), rowStep, 0.05))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_inst;vertical;0.1]")
	iy := 0.05
	for _, inst := range instances {
		b.WriteString(box(0, iy, 13.0, rowH, colorCard))
		b.WriteString(statusDot(0.2, iy+0.16, instanceStatusColor(inst.Status)))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.26, fmtEsc(mcColorize(light, inst.Title()))))

		where := []string{inst.CreatedBy, instanceStatusLabel(inst.Status)}
		if inst.Institute != "" {
			where = append([]string{inst.Institute}, where...)
		}
		if inst.ClassID != nil {
			if name, ok := classNames[*inst.ClassID]; ok {
				where = append(where, T("class @1", name))
			}
		} else {
			where = append(where, T("no class"))
		}
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.58, fmtEsc(mcColorize(muted, strings.Join(where, " · ")))))

		when := T("Created @1", fmtTime(inst.CreatedAt))
		if v, ok := visits[inst.ID]; ok {
			when += " · " + T("last teacher visit @1 (@2)", fmtTime(v.VisitedAt), v.Username)
		} else {
			when += " · " + T("no teacher visit recorded")
		}
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", iy+0.9, fmtEsc(mcColorize(muted, when))))

		b.WriteString(iconBtn(9.6, iy+0.28, 0.6, "info_inst_"+inst.ID, iconPeople, T("Teacher visits")))
		b.WriteString(iconBtn(10.4, iy+0.28, 0.6, "hop_inst_"+inst.ID, iconEnter, T("Enter the world")))
		b.WriteString(iconBtn(11.2, iy+0.28, 0.6, "open_inst_"+inst.ID, iconGear, T("Manage world")))
		delName := "del_inst_" + inst.ID
		if c.isDeleteArmed(cc.Name(), "instance:"+inst.ID) {
			b.WriteString(styledBtn(11.95, iy+0.28, 0.95, 0.6, delName, T("Sure?"), colorDanger))
		} else {
			b.WriteString(iconBtn(12.2, iy+0.28, 0.6, delName, iconClose, T("Delete world")))
		}
		iy += rowStep
	}
	b.WriteString("scroll_container_end[]")
}

// showAdminWorldVisits lists the latest teacher visits to one world.
func (c *controller) showAdminWorldVisits(cc *proxy.ClientConn, inst *instanceData) {
	visits := c.getWorldVisits(inst.ID, visitsKept)

	var b strings.Builder
	dialogFrame(&b, 9, 8.2, accent, iconPeople, inst.Title())
	b.WriteString(box(0.5, 1.5, 8, 5.65, colorCard))
	b.WriteString(sectionTitle(0.8, 1.8, T("TEACHER VISITS")))
	b.WriteString(hint(0.8, 2.2, T("Created @1 by @2", fmtTime(inst.CreatedAt), inst.CreatedBy)))
	if len(visits) == 0 {
		b.WriteString(hint(0.8, 2.8, T("No teacher visit recorded yet.")))
	}
	b.WriteString(scrollbarFor("scr_world_visits", 8.15, 2.5, 4.5, len(visits), 0.45, 0.05))
	b.WriteString("scroll_container[0.8,2.5;7.3,4.5;scr_world_visits;vertical;0.1]")
	for i, v := range visits {
		y := 0.05 + float64(i)*0.45
		b.WriteString(fmt.Sprintf("label[0,%g;%s]", y+0.2, fmtEsc(mcColorize(light, fmtTime(v.VisitedAt)))))
		b.WriteString(fmt.Sprintf("label[2.6,%g;%s]", y+0.2, fmtEsc(v.Username)))
	}
	b.WriteString("scroll_container_end[]")
	b.WriteString(btn(3.0, 7.35, 3.0, 0.65, "btn_back", T("Back")))
	cc.ShowFormspec("classrooms:admin_world", b.String())
}

func (c *controller) writeAdminClassesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institute, teacher := c.getAdminFilters(cc.Name())
	classes, _ := c.getFilteredClasses(institute, teacher)

	c.writeAdminFilters(b, institute, teacher)
	if len(classes) == 0 {
		b.WriteString(hint(0.5, 3.35, T("No classes match the current filters.")))
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
		detail := T("Owner: @1 · @2 students · @3 worlds", cls.CreatedBy, len(students), len(instances))
		if institute, _ := c.getTeacherInstitute(cls.CreatedBy); institute != "" {
			detail = institute + " · " + detail
		}
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.6, fmtEsc(mcColorize(muted, detail))))
		b.WriteString(styledBtn(10.35, y+0.13, 1.6, 0.6, fmt.Sprintf("open_class_%d", cls.ID), T("Open"), colorPrimary))
		delName := fmt.Sprintf("del_class_%d", cls.ID)
		if c.isDeleteArmed(cc.Name(), fmt.Sprintf("class:%d", cls.ID)) {
			b.WriteString(styledBtn(12.05, y+0.13, 0.85, 0.6, delName, T("Sure?"), colorDanger))
		} else {
			b.WriteString(iconBtn(12.2, y+0.13, 0.6, delName, iconClose, T("Delete class")))
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

// adminFilterValues are the choices of the admin institute and teacher
// filters, in dropdown order: "" is "All", then noInstituteFilter for
// institutes, then names.
func (c *controller) adminFilterValues() (institutes, teachers []string) {
	institutes = []string{"", noInstituteFilter}
	if list, err := c.listInstitutes(); err == nil {
		for _, in := range list {
			institutes = append(institutes, in.Name)
		}
	}
	teachers = []string{""}
	names, _ := c.queryStrings(`SELECT username FROM teachers
		UNION SELECT created_by FROM classes
		UNION SELECT created_by FROM instances WHERE status != 'deleted' ORDER BY 1`)
	return institutes, append(teachers, names...)
}

func indexOfString(values []string, v string) int {
	for i, x := range values {
		if x == v {
			return i + 1
		}
	}
	return 1
}

// writeAdminFilters draws the institute and teacher dropdowns of the Worlds
// and Classes tabs. They apply as soon as a choice is made.
func (c *controller) writeAdminFilters(b *strings.Builder, institute, teacher string) {
	institutes, teachers := c.adminFilterValues()
	instItems := append([]string{T("All institutes"), T("No institute")}, institutes[2:]...)
	teacherItems := append([]string{T("All teachers")}, teachers[1:]...)

	b.WriteString(box(0.2, 2.05, 13.55, 0.8, colorCard))
	b.WriteString(fmt.Sprintf("image[0.4,2.22;0.45,0.45;%s]", iconSearch))
	b.WriteString(hint(1.0, 2.45, T("Institute")))
	b.WriteString(dropdownOf(2.1, 2.2, 3.4, 0.5, "admin_filter_institute", instItems, indexOfString(institutes, institute)))
	b.WriteString(hint(5.75, 2.45, T("Teacher")))
	b.WriteString(dropdownOf(6.9, 2.2, 3.4, 0.5, "admin_filter_teacher", teacherItems, indexOfString(teachers, teacher)))
	b.WriteString(btn(10.6, 2.2, 2.4, 0.5, "btn_admin_filter_clear", T("Clear filters")))
}

func (c *controller) writeAdminInstitutesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institutes, _ := c.listInstitutes()

	b.WriteString(box(0.2, 2.05, 13.55, 0.8, colorCard))
	b.WriteString(fmt.Sprintf("image[0.4,2.22;0.45,0.45;%s]", iconPlus))
	b.WriteString("field[1.0,2.2;4.3,0.5;new_institute_name;;]")
	b.WriteString(tooltip("new_institute_name", T("Institute name")))
	b.WriteString("field[6.8,2.2;0.9,0.5;new_institute_max_classes;;0]")
	b.WriteString(tooltip("new_institute_max_classes", T("Maximum classes of the institute; 0 means no limit.")))
	b.WriteString("field[8.8,2.2;0.9,0.5;new_institute_max_worlds;;0]")
	b.WriteString(tooltip("new_institute_max_worlds", T("Maximum worlds per class; 0 means no limit.")))
	b.WriteString(styledBtn(10.2, 2.2, 1.6, 0.5, "btn_admin_add_institute", T("Add"), colorPrimary))

	// Column titles, aligned with the fields of the rows below.
	b.WriteString(hint(0.4, 3.15, T("Institute")))
	b.WriteString(hint(3.8, 3.15, T("Usage")))
	b.WriteString(hint(6.8, 3.15, T("Max classes")))
	b.WriteString(hint(8.8, 3.15, T("Worlds per class")))
	b.WriteString(hint(11.95, 2.45, T("0 = no limit")))
	if len(institutes) == 0 {
		b.WriteString(hint(0.5, 3.8, T("No institutes yet: add one above.")))
		return
	}

	b.WriteString(scrollbarFor("scr_admin_institutes", 13.45, 3.4, 4.85, len(institutes), 0.95, 0.05))
	b.WriteString("scroll_container[0.2,3.4;13.15,4.85;scr_admin_institutes;vertical;0.1]")
	y := 0.05
	for _, in := range institutes {
		id := fmt.Sprint(in.ID)
		b.WriteString(box(0, y, 13.0, 0.85, colorCard))
		b.WriteString(fmt.Sprintf("field[0.2,%g;3.2,0.5;institute_name_%s;;%s]", y+0.17, id, fmtEsc(in.Name)))
		usage := T("@1 teachers · @2 classes", in.Teachers, in.Classes)
		if in.MaxClasses > 0 {
			usage = T("@1 teachers · @2/@3 classes", in.Teachers, in.Classes, in.MaxClasses)
		}
		b.WriteString(fmt.Sprintf("label[3.6,%g;%s]", y+0.42, fmtEsc(mcColorize(muted, usage))))
		b.WriteString(fmt.Sprintf("field[6.6,%g;0.9,0.5;institute_max_classes_%s;;%d]", y+0.17, id, in.MaxClasses))
		b.WriteString(fmt.Sprintf("field[8.6,%g;0.9,0.5;institute_max_worlds_%s;;%d]", y+0.17, id, in.MaxWorldsPerClass))
		b.WriteString(btn(10.0, y+0.14, 1.4, 0.58, "save_institute_"+id, T("Save")))
		delName := "del_institute_" + id
		if c.isDeleteArmed(cc.Name(), "institute:"+id) {
			b.WriteString(styledBtn(11.95, y+0.13, 0.95, 0.6, delName, T("Sure?"), colorDanger))
			b.WriteString(tooltip(delName, T("Also deletes its classes and their worlds; its teachers become normal users.")))
		} else {
			b.WriteString(iconBtn(12.2, y+0.13, 0.6, delName, iconClose,
				T("Delete the institute, its classes and their worlds")))
		}
		y += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

// ── Password dialog ─────────────────────────────────────────────────────────

func (c *controller) showPasswordDialog(cc *proxy.ClientConn) {
	target, ok := c.getPasswordTarget(cc.Name())
	if !ok || !c.canChangePassword(cc.Name(), target.Name) {
		c.closePasswordDialog(cc)
		return
	}

	var b strings.Builder
	dialogFrame(&b, 9, 6.4, accent, iconLock, T("Change password"))
	b.WriteString(box(0.5, 1.6, 8, 3.55, colorCard))
	b.WriteString(coloredLbl(0.85, 2.0, light, T("Account: @1", target.Name)))
	b.WriteString(hint(0.85, 2.45, T("The new password applies from the next login.")))
	b.WriteString("pwdfield[0.85,3.15;3.6,0.65;new_password;" + fmtEsc(T("New password (8+ chars)")) + "]")
	b.WriteString("pwdfield[4.55,3.15;3.6,0.65;new_password_confirm;" + fmtEsc(T("Repeat password")) + "]")
	b.WriteString("field_close_on_enter[new_password;false]")
	b.WriteString("field_close_on_enter[new_password_confirm;false]")
	b.WriteString(styledBtn(0.85, 5.45, 3.6, 0.7, "btn_password_save", T("Change password"), colorPrimary))
	b.WriteString(btn(4.55, 5.45, 3.6, 0.7, "btn_password_cancel", T("Cancel")))
	cc.ShowFormspec("classrooms:password", b.String())
}

// ── People editor: Students | Assistance | Teachers ─────────────────────────

const (
	peopleTabStudents   = "tab_students"
	peopleTabAssistance = "tab_assistance"
	peopleTabTeachers   = "tab_teachers"
)

// Students and Groups use the large two-column frame.
const peopleWideW, peopleWideH = 17.0, 11.6

func (c *controller) peopleFrame(b *strings.Builder, cc *proxy.ClientConn, cls *classData, activeTab string) {
	c.peopleFrameSize(b, cc, cls, activeTab, 9, 9.6)
}

func (c *controller) peopleFrameSize(b *strings.Builder, cc *proxy.ClientConn, cls *classData, activeTab string, w, h float64) {
	fsOpen(b, w, h)
	fsHeader(b, w, T("People"), cls.Name, true, true)
	tabs := [][2]string{{peopleTabStudents, T("Students")}}
	if c.canManageClass(cls.ID, cc.Name()) {
		tabs = append(tabs, [2]string{peopleTabGroups, T("Groups")},
			[2]string{peopleTabAssistance, T("Assistance")}, [2]string{peopleTabTeachers, T("Teachers")})
	}
	tabBar(b, 0.3, 1.2, 1.95, activeTab, tabs)
}

// personList draws a scrollable list of names with a remove button each.
func personList(b *strings.Builder, y, h float64, names []string, removePrefix, removeTip string) {
	if len(names) == 0 {
		b.WriteString(hint(0.6, y+0.4, T("Nobody yet.")))
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

// showStudentEditor: adding options on the left, the searchable class list
// (with each student's group) on the right.
func (c *controller) showStudentEditor(cc *proxy.ClientConn, classID int) {
	cls, _ := c.getClassByID(classID)
	if cls == nil || !c.canEditClassStudents(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	students, _ := c.getStudents(classID)
	groups, _ := c.getGroups(classID)
	byStudent := groupByStudent(groups)
	view := c.studentList(cc.Name(), classID)
	isAdmin := c.isAdmin(cc.Name())
	created := c.getAccountCreators(cc.Name())

	var b strings.Builder
	c.peopleFrameSize(&b, cc, cls, peopleTabStudents, peopleWideW, peopleWideH)

	// ── Left: add students ──
	b.WriteString(box(0.3, 2.05, 6.6, 2.05, colorCard))
	b.WriteString(fmt.Sprintf("image[0.55,2.2;0.42,0.42;%s]", iconPlus))
	b.WriteString(sectionTitle(1.1, 2.42, T("ADD AN EXISTING PLAYER")))
	b.WriteString(hint(0.55, 2.85, T("Type the username they log in with.")))
	b.WriteString("field[0.55,3.15;4.25,0.65;add_student_name;;]")
	b.WriteString("field_close_on_enter[add_student_name;false]")
	b.WriteString(styledBtn(4.95, 3.15, 1.7, 0.65, "btn_add_student", T("+ Add"), colorPrimary))

	statsTop := 4.3
	if c.canManageClass(classID, cc.Name()) {
		b.WriteString(box(0.3, 4.3, 6.6, 4.35, colorCard))
		b.WriteString(fmt.Sprintf("image[0.55,4.45;0.42,0.42;%s]", iconPlayer))
		b.WriteString(sectionTitle(1.1, 4.67, T("CREATE A NEW ACCOUNT")))
		b.WriteString(hint(0.55, 5.1, T("For students who have never played here.")))
		b.WriteString("field[0.55,5.75;6.1,0.65;new_student_name;" + fmtEsc(T("Username")) + ";]")
		b.WriteString("pwdfield[0.55,6.85;2.97,0.65;new_student_password;" + fmtEsc(T("Password (8+ chars)")) + "]")
		b.WriteString("pwdfield[3.68,6.85;2.97,0.65;new_student_confirm;" + fmtEsc(T("Repeat password")) + "]")
		b.WriteString(styledBtn(0.55, 7.75, 6.1, 0.7, "btn_create_student", T("Create and add to the class"), colorPrimary))
		for _, field := range []string{"new_student_name", "new_student_password", "new_student_confirm"} {
			b.WriteString("field_close_on_enter[" + field + ";false]")
		}
		statsTop = 8.85
	}

	// Class summary.
	online, grouped := 0, 0
	for _, s := range students {
		if proxy.Find(s) != nil {
			online++
		}
		if _, ok := byStudent[s]; ok {
			grouped++
		}
	}
	b.WriteString(box(0.3, statsTop, 6.6, 11.3-statsTop, colorCard))
	b.WriteString(sectionTitle(0.55, statsTop+0.3, T("THIS CLASS")))
	lines := []string{T("@1 students", len(students)), T("@1 online now", online)}
	if len(groups) > 0 {
		lines = append(lines, T("@1 groups · @2 without a group", len(groups), len(students)-grouped))
	}
	for i, line := range lines {
		if statsTop+0.75+float64(i)*0.45 > 11.1 {
			break
		}
		b.WriteString(coloredLbl(0.55, statsTop+0.75+float64(i)*0.45, light, line))
	}

	// ── Right: the class list ──
	b.WriteString(box(7.1, 2.05, 9.6, 9.25, colorCard))
	b.WriteString(sectionTitle(7.35, 2.35, T("IN THIS CLASS (@1)", len(students))))
	b.WriteString(fmt.Sprintf("field[7.35,2.6;5.6,0.6;stu_search;;%s]", fmtEsc(view.PeopleSearch)))
	b.WriteString("field_close_on_enter[stu_search;false]")
	b.WriteString(tooltip("stu_search", T("Search by name, then press Enter")))
	b.WriteString(iconBtn(13.05, 2.6, 0.6, "stu_search_go", iconSearch, T("Search")))
	b.WriteString(iconBtn(13.75, 2.6, 0.6, "stu_search_clear", iconClose, T("Clear the search")))

	shown := filterStudents(students, byStudent, "all", view.PeopleSearch)
	if view.PeopleSearch != "" {
		b.WriteString(hint(14.5, 2.9, fmt.Sprintf("%d of %d", len(shown), len(students))))
	}
	if len(shown) == 0 {
		msg := T("No students match the search.")
		if len(students) == 0 {
			msg = T("No students yet: add them on the left.")
		}
		b.WriteString(hint(7.4, 3.75, msg))
	}
	const listTop, listH, rowStep = 3.4, 7.75, 0.75
	b.WriteString(scrollbarAt("scr_people_students", 16.35, listTop, listH, len(shown), rowStep, 0.05, view.Scroll["scr_people_students"]))
	b.WriteString(fmt.Sprintf("scroll_container[7.25,%g;9.0,%g;scr_people_students;vertical;0.1]", listTop, listH))
	ry := 0.05
	for _, s := range shown {
		b.WriteString(box(0, ry, 8.95, 0.68, colorRow))
		dot, state := muted, T("offline")
		if proxy.Find(s) != nil {
			dot, state = success, T("online")
		}
		b.WriteString(statusDot(0.2, ry+0.23, dot))
		b.WriteString(fmt.Sprintf("label[0.6,%g;%s]", ry+0.34, fmtEsc(s)))
		b.WriteString(fmt.Sprintf("label[3.9,%g;%s]", ry+0.34, fmtEsc(mcColorize(muted, state))))
		if g, ok := byStudent[s]; ok {
			b.WriteString(box(5.2, ry+0.23, 0.22, 0.22, g.Color))
			name := []rune(g.Name)
			if len(name) > 13 {
				name = append(name[:12], '…')
			}
			b.WriteString(fmt.Sprintf("label[5.55,%g;%s]", ry+0.34, fmtEsc(mcColorize(g.Color, string(name)))))
		} else if len(groups) > 0 {
			b.WriteString(fmt.Sprintf("label[5.55,%g;%s]", ry+0.34, fmtEsc(mcColorize(muted, T("no group")))))
		}
		if _, mine := created[s]; isAdmin || (mine && c.canChangePassword(cc.Name(), s)) {
			b.WriteString(iconBtn(7.7, ry+0.09, 0.5, "pw_student_"+s, iconLock, T("Change password: @1", s)))
		}
		b.WriteString(iconBtn(8.3, ry+0.09, 0.5, "rm_student_"+s, iconClose, T("Remove from class: @1", s)))
		ry += rowStep
	}
	b.WriteString("scroll_container_end[]")

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
	explain := T("Co-teachers have the same controls as you.")
	if assistance {
		members, _ = c.getClassAssistants(classID)
		tab, formName, fieldName, addButton, removePrefix = peopleTabAssistance, "classrooms:assistants", "add_assistance_name", "btn_add_assistance", "rm_assistance_"
		explain = T("Assistance can add students and join class worlds.")
	}

	var b strings.Builder
	c.peopleFrame(&b, cc, cls, tab)
	b.WriteString(box(0.3, 2.05, 8.4, 1.6, colorCard))
	b.WriteString(hint(0.55, 2.35, explain))
	b.WriteString(fmt.Sprintf("field[0.55,2.85;5.6,0.55;%s;;]", fieldName))
	b.WriteString(fmt.Sprintf("field_close_on_enter[%s;false]", fieldName))
	b.WriteString(styledBtn(6.35, 2.85, 2.1, 0.55, addButton, T("+ Add"), colorPrimary))
	listTop := 4.25
	if !assistance {
		b.WriteString(hint(0.45, 3.95, T("Owner: @1", cls.CreatedBy)))
		listTop = 4.45
	} else {
		b.WriteString(sectionTitle(0.45, 3.95, T("ASSISTANCE (@1)", len(members))))
	}
	personList(&b, listTop, 9.4-listTop, members, removePrefix, T("Remove"))
	cc.ShowFormspec(formName, b.String())
}
