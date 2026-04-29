package main

import (
	"fmt"
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

// ── Main Dashboard (Class List) ─────────────────────────────────────────────

func (c *controller) showMainDashboard(cc *proxy.ClientConn) {
	name := cc.Name()
	classes, err := c.getClasses(name)
	if err != nil {
		cc.SendChatMsg("[Classrooms] Error loading classes: " + err.Error())
		return
	}

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[12,8.6]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	// Header
	b.WriteString(box(0, 0, 12, 0.85, panel))
	b.WriteString(coloredLbl(0.35, 0.38, accent, "| "))
	b.WriteString(coloredLbl(0.75, 0.38, light, "Classrooms - Teacher Dashboard"))
	if c.isAdmin(name) {
		b.WriteString(btn(8.85, 0.15, 1.45, 0.52, "btn_admin_panel", "Admin"))
	}
	b.WriteString(btnExit(10.45, 0.15, 1.25, 0.52, "btn_close", "Close"))
	b.WriteString(box(0, 0.85, 12, 0.04, accent))

	// Create class section
	b.WriteString(box(0.3, 1.15, 11.4, 0.75, panel))
	b.WriteString(coloredLbl(0.55, 1.48, muted, "New class"))
	b.WriteString("field[1.9,1.28;6.8,0.5;new_class_name;;]")
	b.WriteString("field_close_on_enter[new_class_name;false]")
	b.WriteString(btn(9.0, 1.27, 1.3, 0.52, "btn_create_class", "+ Create"))

	// List header
	b.WriteString(coloredLbl(0.35, 2.35, muted, "YOUR CLASSES"))

	if len(classes) == 0 {
		b.WriteString(coloredLbl(0.5, 3.2, muted, "No classes yet. Create one above to get started."))
	} else {
		b.WriteString(scrollbarFor("scr_classes", 11.45, 2.75, 5.45, len(classes), 0.95, 0.05))
		b.WriteString("scroll_container[0.2,2.75;11.15,5.45;scr_classes;vertical;0.1]")
		y := 0.05
		for _, cls := range classes {
			students, _ := c.getStudents(cls.ID)
			online := 0
			for _, s := range students {
				if proxy.Find(s) != nil {
					online++
				}
			}

			b.WriteString(box(0, y, 11.0, 0.85, panel))
			b.WriteString(fmt.Sprintf("label[0.25,%g;%s]", y+0.32,
				fmtEsc(mcColorize(light, cls.Name)+
					mcColorize(muted, fmt.Sprintf("  (%d/%d online)", online, len(students))))))

			b.WriteString(fmt.Sprintf("button[8.2,%g;1.0,0.55;open_class_%d;Open]", y+0.14, cls.ID))
			b.WriteString(fmt.Sprintf("button[9.35,%g;1.0,0.55;del_class_%d;%s]", y+0.14, cls.ID, fmtEsc(mcColorize(danger, "Del"))))
			y += 0.95
		}
		b.WriteString("scroll_container_end[]")
	}

	cc.ShowFormspec("classrooms:main", b.String())
}

// ── Class View (Students + Instances) ───────────────────────────────────────

func (c *controller) showClassView(cc *proxy.ClientConn, classID int) {
	c.showClassViewWithOrigin(cc, classID, viewOriginTeacher)
}

func (c *controller) showClassViewWithOrigin(cc *proxy.ClientConn, classID int, origin string) {
	if origin == "" {
		origin = viewOriginTeacher
	}
	cls, err := c.getClassByID(classID)
	if err != nil || cls == nil {
		c.showClassFallback(cc, origin)
		return
	}
	c.setActiveClassWithOrigin(cc.Name(), classID, origin)

	students, _ := c.getStudents(classID)
	instances, _ := c.getInstancesForClass(classID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[14,8.6]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	// Header
	b.WriteString(box(0, 0, 14, 0.85, panel))
	b.WriteString(btn(0.2, 0.15, 1.25, 0.52, "btn_back", "Back"))
	b.WriteString(coloredLbl(1.75, 0.38, accent, "| "))
	b.WriteString(coloredLbl(2.15, 0.38, light, "Class: "+cls.Name))
	if origin == viewOriginAdminClasses {
		b.WriteString(coloredLbl(8.0, 0.38, muted, "Owner: "+cls.CreatedBy))
	}
	b.WriteString(btnExit(12.45, 0.15, 1.25, 0.52, "btn_close", "Close"))
	b.WriteString(box(0, 0.85, 14, 0.04, accent))

	// Left Column: Student Controls (6 units wide)
	b.WriteString(coloredLbl(0.35, 1.25, muted, "STUDENTS"))
	b.WriteString(box(0.2, 1.55, 6.55, 6.75, panel))

	b.WriteString(btn(0.4, 1.75, 1.9, 0.5, "btn_freeze_all", "Freeze"))
	b.WriteString(btn(2.45, 1.75, 1.9, 0.5, "btn_unfreeze_all", "Unfreeze"))
	b.WriteString(btn(4.5, 1.75, 1.9, 0.5, "btn_gather_all", "Gather"))
	b.WriteString(btn(0.4, 2.35, 1.9, 0.5, "btn_watch_teacher", "Watch"))
	b.WriteString(btn(2.45, 2.35, 1.9, 0.5, "btn_stop_watching", "Stop Watch"))
	b.WriteString(btn(4.5, 2.35, 1.9, 0.5, "btn_manage_students", "Edit Names"))

	b.WriteString(scrollbarFor("scr_students", 6.3, 3.1, 4.95, len(students), 0.75, 0.05))
	b.WriteString("scroll_container[0.4,3.1;5.8,4.95;scr_students;vertical;0.1]")
	sy := 0.05
	for _, s := range students {
		online := proxy.Find(s) != nil
		statusColor := muted
		if online {
			statusColor = success
		}

		b.WriteString(box(0, sy, 5.65, 0.65, headerColor))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", sy+0.25, fmtEsc(mcColorize(statusColor, s))))

		if online {
			b.WriteString(fmt.Sprintf("button[3.45,%g;0.8,0.45;tp_to_%s;TP]", sy+0.1, fmtEsc(s)))
			b.WriteString(fmt.Sprintf("button[4.35,%g;0.8,0.45;watch_%s;Eye]", sy+0.1, fmtEsc(s)))
		}
		sy += 0.75
	}
	b.WriteString("scroll_container_end[]")

	// Right Column: Instances (7 units wide)
	b.WriteString(coloredLbl(7.05, 1.25, muted, "INSTANCES"))
	b.WriteString(box(6.95, 1.55, 6.85, 6.75, panel))
	b.WriteString(btn(7.15, 1.75, 6.35, 0.55, "btn_create_instance", "+ Provision New Instance"))

	b.WriteString(scrollbarFor("scr_instances", 13.45, 2.55, 5.5, len(instances), 0.95, 0.05))
	b.WriteString("scroll_container[7.15,2.55;6.2,5.5;scr_instances;vertical;0.1]")
	iy := 0.05
	for _, inst := range instances {
		statusColor := muted
		switch inst.Status {
		case "running":
			statusColor = success
		case "provisioning":
			statusColor = warning
		case "stopped":
			statusColor = danger
		}

		b.WriteString(box(0, iy, 6.05, 0.85, headerColor))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.28, fmtEsc(mcColorize(light, inst.Title()))))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.58, fmtEsc(mcColorize(muted, inst.TemplateName+" | ")+mcColorize(statusColor, inst.Status))))

		b.WriteString(fmt.Sprintf("button[4.75,%g;1.0,0.55;open_inst_%s;Open]", iy+0.14, fmtEsc(inst.ID)))
		iy += 0.95
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:class", b.String())
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
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,7.2]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 0.9, panel))
	b.WriteString(btn(0.2, 0.15, 1.3, 0.55, "btn_back", "Back"))
	b.WriteString(coloredLbl(1.8, 0.42, light, "Create Instance"))
	b.WriteString(box(0, 0.9, 10, 0.04, accent))

	b.WriteString(box(0.4, 1.15, 9.2, 0.9, panel))
	b.WriteString(coloredLbl(0.65, 1.53, muted, "Name"))
	b.WriteString("field[1.7,1.4;7.4,0.55;new_instance_name;;]")
	b.WriteString("field_close_on_enter[new_instance_name;false]")

	b.WriteString(scrollbarFor("scr_templates", 9.25, 2.25, 4.55, len(templates), 1.0, 0.05))
	b.WriteString("scroll_container[0.4,2.25;8.75,4.55;scr_templates;vertical;0.1]")
	y := 0.05
	for _, tName := range templates {
		tpl := c.cfg.Templates[tName]
		b.WriteString(box(0, y, 8.55, 0.9, panel))
		b.WriteString(fmt.Sprintf("label[0.25,%g;%s]", y+0.32, fmtEsc(mcColorize(light, tName))))
		b.WriteString(fmt.Sprintf("label[0.25,%g;%s]", y+0.62, fmtEsc(mcColorize(muted, tpl.ServerDescription))))
		b.WriteString(fmt.Sprintf("button[6.75,%g;1.4,0.55;pick_tpl_%s;Select]", y+0.17, fmtEsc(tName)))
		y += 1.0
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:template_picker", b.String())
}

// ── Instance Operation Dialogs ──────────────────────────────────────────────

func (c *controller) showInstanceProgress(cc *proxy.ClientConn, title, detail string) {
	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[9,5.5]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 9, 1.1, panel))
	b.WriteString(coloredLbl(0.5, 0.5, warning, "| "))
	b.WriteString(coloredLbl(0.9, 0.5, light, title))
	b.WriteString(box(0, 1.1, 9, 0.05, warning))

	b.WriteString(box(0.5, 1.6, 8, 2.7, panel))
	b.WriteString(coloredLbl(0.9, 2.2, light, detail))
	b.WriteString(coloredLbl(0.9, 2.9, muted, "You can close this window. The operation will keep running."))
	b.WriteString(coloredLbl(0.9, 3.5, muted, "A new window will open when the server is ready."))

	b.WriteString(btnExit(3.0, 4.5, 3.0, 0.7, "btn_progress_close", "Close"))
	cc.ShowFormspec("classrooms:instance_progress", b.String())
}

func (c *controller) showInstanceReady(cc *proxy.ClientConn, inst *instanceData, title string) {
	if inst == nil {
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
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,7]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 1.1, panel))
	b.WriteString(coloredLbl(0.5, 0.5, success, "| "))
	b.WriteString(coloredLbl(0.9, 0.5, light, title))
	b.WriteString(box(0, 1.1, 10, 0.05, success))

	b.WriteString(box(0.5, 1.6, 9, 2.3, panel))
	b.WriteString(coloredLbl(0.9, 2.1, light, inst.Title()))
	b.WriteString(coloredLbl(0.9, 2.7, muted, "Template: "+inst.TemplateName))
	b.WriteString(coloredLbl(0.9, 3.3, muted, "The server is running and available in the proxy."))

	b.WriteString(box(0.5, 4.3, 9, 1.8, panel))
	b.WriteString(btn(0.9, 4.7, 2.6, 0.8, "btn_ready_hop_me", "Hop Me Here"))
	if inst.ClassID != nil {
		b.WriteString(btn(3.7, 4.7, 2.6, 0.8, "btn_ready_hop_class", "Bring Class"))
	}
	b.WriteString(btn(6.5, 4.7, 2.6, 0.8, "btn_ready_open", "Manage"))
	b.WriteString(btnExit(3.7, 6.2, 2.6, 0.6, "btn_ready_close", "Close"))

	cc.ShowFormspec("classrooms:instance_ready", b.String())
}

func (c *controller) showInstanceError(cc *proxy.ClientConn, inst *instanceData, title, detail string) {
	if inst != nil {
		c.setActiveInstance(cc.Name(), inst.ID)
	}

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,7]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 1.1, panel))
	b.WriteString(coloredLbl(0.5, 0.5, danger, "| "))
	b.WriteString(coloredLbl(0.9, 0.5, light, title))
	b.WriteString(box(0, 1.1, 10, 0.05, danger))

	b.WriteString(box(0.5, 1.6, 9, 3.3, panel))
	b.WriteString(coloredLbl(0.9, 2.1, light, "The server operation did not complete."))
	b.WriteString(fmt.Sprintf("textarea[0.9,2.7;8.2,1.7;error_detail;;%s]", fmtEsc(detail)))

	if inst != nil {
		b.WriteString(btn(1.0, 5.4, 2.6, 0.8, "btn_error_open", "Manage"))
	}
	b.WriteString(btnExit(6.4, 5.4, 2.6, 0.8, "btn_error_close", "Close"))

	cc.ShowFormspec("classrooms:instance_error", b.String())
}

// ── Instance Detail View ────────────────────────────────────────────────────

func (c *controller) showInstanceView(cc *proxy.ClientConn, instanceID string) {
	c.showInstanceViewWithOrigin(cc, instanceID, viewOriginTeacher)
}

func (c *controller) showInstanceViewWithOrigin(cc *proxy.ClientConn, instanceID, origin string) {
	if origin == "" {
		origin = viewOriginTeacher
	}
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil {
		c.showInstanceFallback(cc, origin)
		return
	}
	c.setActiveInstanceWithOrigin(cc.Name(), instanceID, origin)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,10]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.5, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(2.0, 0.5, light, "Instance: "+inst.Title()))
	b.WriteString(box(0, 1.1, 10, 0.05, accent))

	// Info Box
	b.WriteString(box(0.5, 1.5, 9, 3, panel))
	b.WriteString(coloredLbl(0.8, 2.0, muted, "Template: "+inst.TemplateName))
	b.WriteString(coloredLbl(0.8, 2.5, muted, "Creator:  "+inst.CreatedBy))
	if inst.Institute != "" {
		b.WriteString(coloredLbl(5.0, 2.5, muted, "Institute: "+inst.Institute))
	}

	statusColor := muted
	switch inst.Status {
	case "running":
		statusColor = success
	case "provisioning":
		statusColor = warning
	case "stopped":
		statusColor = danger
	}
	b.WriteString(fmt.Sprintf("label[0.8,3.0;%s]", fmtEsc(mcColorize(muted, "Status:   ")+mcColorize(statusColor, inst.Status))))
	b.WriteString(coloredLbl(0.8, 3.5, muted, "Address:  "+inst.ProxyName))
	if inst.DisplayName != "" {
		b.WriteString(coloredLbl(0.8, 4.0, muted, "ID:       "+inst.ID))
	}

	// Controls
	b.WriteString(box(0.5, 4.8, 9, 4.5, panel))

	if inst.Status == "running" {
		b.WriteString(btn(1.0, 5.2, 3.5, 0.8, "btn_inst_stop", "Stop Server"))
		b.WriteString(btn(5.5, 5.2, 3.5, 0.8, "btn_hop_me", "Hop Me Here"))
		if inst.ClassID != nil {
			b.WriteString(btn(5.5, 6.2, 3.5, 0.8, "btn_hop_class", "Hop Class Here"))
		}
	} else if inst.Status == "stopped" {
		b.WriteString(btn(1.0, 5.2, 3.5, 0.8, "btn_inst_start", "Start Server"))
	}

	b.WriteString(btn(1.0, 8.2, 3.5, 0.8, "btn_inst_delete", mcColorize(danger, "Delete Instance")))

	// Invites
	b.WriteString(coloredLbl(5.5, 7.2, muted, "Invites (Teacher only):"))
	b.WriteString("field[5.5,7.7;2.5,0.7;invite_name;;]")
	b.WriteString(btn(8.1, 7.7, 0.9, 0.7, "btn_invite", "+"))

	cc.ShowFormspec("classrooms:instance", b.String())
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
	b.WriteString("formspec_version[6]")
	b.WriteString("size[14,8.6]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 14, 0.85, panel))
	b.WriteString(coloredLbl(0.4, 0.38, accent, "| "))
	b.WriteString(coloredLbl(0.8, 0.38, light, "Classrooms - GLOBAL ADMIN"))
	b.WriteString(btnExit(12.45, 0.15, 1.25, 0.52, "btn_close", "Close"))
	b.WriteString(box(0, 0.85, 14, 0.04, danger))

	b.WriteString(btn(0.35, 1.05, 1.7, 0.5, "btn_admin_tab_instances", tabLabel(activeTab == "instances", "Instances")))
	b.WriteString(btn(2.15, 1.05, 1.7, 0.5, "btn_admin_tab_classes", tabLabel(activeTab == "classes", "Classes")))
	b.WriteString(btn(3.95, 1.05, 1.7, 0.5, "btn_admin_tab_teachers", tabLabel(activeTab == "teachers", "Teachers")))

	if activeTab == "teachers" {
		c.writeAdminTeachersTab(&b)
	} else if activeTab == "classes" {
		c.writeAdminClassesTab(cc, &b)
	} else {
		c.writeAdminInstancesTab(cc, &b)
	}

	cc.ShowFormspec("classrooms:admin", b.String())
}

func tabLabel(active bool, label string) string {
	if active {
		return mcColorize(accent, label)
	}
	return label
}

func (c *controller) writeAdminInstancesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institute, teacher := c.getAdminFilters(cc.Name())
	instances, _ := c.getFilteredActiveInstances(institute, teacher)

	b.WriteString(coloredLbl(0.35, 1.85, muted, "ALL INSTANCES"))
	c.writeAdminFilters(b, institute, teacher)
	b.WriteString(adminScroll("scr_admin_inst", len(instances)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_inst;vertical;0.1]")
	iy := 0.05
	for _, inst := range instances {
		b.WriteString(box(0, iy, 13.0, 0.85, panel))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.28, fmtEsc(mcColorize(light, inst.Title()))))
		detail := inst.CreatedBy + " | " + inst.Status
		if inst.Institute != "" {
			detail = inst.Institute + " | " + detail
		}
		if inst.DisplayName != "" {
			detail = detail + " | " + inst.ID
		}
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.58, fmtEsc(mcColorize(muted, detail))))
		b.WriteString(fmt.Sprintf("button[11.55,%g;1.0,0.55;open_inst_%s;Open]", iy+0.14, fmtEsc(inst.ID)))
		iy += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

func (c *controller) writeAdminClassesTab(cc *proxy.ClientConn, b *strings.Builder) {
	institute, teacher := c.getAdminFilters(cc.Name())
	classes, _ := c.getFilteredClasses(institute, teacher)

	b.WriteString(coloredLbl(0.35, 1.85, muted, "ALL CLASSES"))
	c.writeAdminFilters(b, institute, teacher)
	if len(classes) == 0 {
		b.WriteString(coloredLbl(0.5, 3.35, muted, "No classes match the current filters."))
		return
	}

	b.WriteString(adminScroll("scr_admin_classes", len(classes)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_classes;vertical;0.1]")
	y := 0.05
	for _, cls := range classes {
		students, _ := c.getStudents(cls.ID)
		instances, _ := c.getInstancesForClass(cls.ID)
		b.WriteString(box(0, y, 13.0, 0.85, panel))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.28, fmtEsc(mcColorize(light, cls.Name))))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.58, fmtEsc(mcColorize(muted,
			fmt.Sprintf("Owner: %s | Students: %d | Instances: %d", cls.CreatedBy, len(students), len(instances))))))
		b.WriteString(fmt.Sprintf("button[10.65,%g;0.9,0.55;open_class_%d;Open]", y+0.14, cls.ID))
		b.WriteString(fmt.Sprintf("button[11.7,%g;1.0,0.55;del_class_%d;%s]", y+0.14, cls.ID, fmtEsc(mcColorize(danger, "Del"))))
		y += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

func adminScroll(name string, itemCount int) string {
	return scrollbarFor(name, 13.45, 3.0, 5.25, itemCount, 0.95, 0.05)
}

func scrollbarFor(name string, x, y, h float64, itemCount int, rowStep, topPad float64) string {
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
	return fmt.Sprintf("scrollbaroptions[min=0;max=%d;smallstep=4;largestep=16;arrows=default]scrollbar[%g,%g;0.25,%g;vertical;%s;0]", max, x, y, h, name)
}

func (c *controller) writeAdminFilters(b *strings.Builder, institute, teacher string) {
	b.WriteString(box(0.2, 2.12, 13.55, 0.72, panel))
	b.WriteString(coloredLbl(0.45, 2.46, muted, "Institute"))
	b.WriteString("field[1.55,2.25;3.35,0.45;admin_filter_institute;;")
	b.WriteString(fmtEsc(institute))
	b.WriteString("]")
	b.WriteString(coloredLbl(5.15, 2.46, muted, "Teacher"))
	b.WriteString("field[6.05,2.25;3.35,0.45;admin_filter_teacher;;")
	b.WriteString(fmtEsc(teacher))
	b.WriteString("]")
	b.WriteString(btn(9.85, 2.25, 1.1, 0.5, "btn_admin_filter_apply", "Filter"))
	b.WriteString(btn(11.1, 2.25, 1.1, 0.5, "btn_admin_filter_clear", "Clear"))
}

func (c *controller) writeAdminTeachersTab(b *strings.Builder) {
	teachers, _ := c.listTeacherRecords()

	b.WriteString(coloredLbl(0.35, 1.85, muted, "TEACHERS"))
	b.WriteString(box(0.2, 2.12, 13.55, 0.72, panel))
	b.WriteString(coloredLbl(0.45, 2.46, muted, "Username"))
	b.WriteString("field[1.65,2.25;3.35,0.45;new_teacher_name;;]")
	b.WriteString(coloredLbl(5.15, 2.46, muted, "Institute"))
	b.WriteString("field[6.05,2.25;3.35,0.45;new_teacher_institute;;]")
	b.WriteString(btn(9.85, 2.25, 1.1, 0.5, "btn_admin_add_teacher", "Add"))

	b.WriteString(adminScroll("scr_admin_teachers", len(teachers)))
	b.WriteString("scroll_container[0.2,3.0;13.15,5.25;scr_admin_teachers;vertical;0.1]")
	ty := 0.05
	for _, t := range teachers {
		fieldName := "teacher_institute_" + t.Username
		b.WriteString(box(0, ty, 13.0, 0.85, panel))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", ty+0.32, fmtEsc(t.Username)))
		b.WriteString(fmt.Sprintf("field[4.1,%g;5.0,0.45;%s;;%s]", ty+0.14, fmtEsc(fieldName), fmtEsc(t.Institute)))
		b.WriteString(fmt.Sprintf("button[9.45,%g;1.0,0.55;save_teacher_%s;Save]", ty+0.14, fmtEsc(t.Username)))
		b.WriteString(fmt.Sprintf("button[10.6,%g;0.9,0.55;rm_teacher_%s;%s]", ty+0.14, fmtEsc(t.Username), fmtEsc(mcColorize(danger, "Rm"))))
		ty += 0.95
	}
	b.WriteString("scroll_container_end[]")
}

// ── Student Names List ──────────────────────────────────────────────────────

func (c *controller) showStudentEditor(cc *proxy.ClientConn, classID int) {
	cls, _ := c.getClassByID(classID)
	students, _ := c.getStudents(classID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[8,8.6]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 8, 0.85, panel))
	b.WriteString(btn(0.2, 0.15, 1.2, 0.52, "btn_back", "Back"))
	b.WriteString(coloredLbl(1.6, 0.38, light, "Students: "+cls.Name))
	b.WriteString(btnExit(6.55, 0.15, 1.2, 0.52, "btn_close", "Close"))
	b.WriteString(box(0, 0.85, 8, 0.04, accent))

	b.WriteString(box(0.25, 1.15, 7.5, 0.72, panel))
	b.WriteString("field[0.55,1.28;5.3,0.5;add_student_name;;]")
	b.WriteString(btn(6.05, 1.27, 1.2, 0.52, "btn_add_student", "Add"))

	b.WriteString(scrollbarFor("scr_edit_students", 7.35, 2.1, 6.1, len(students), 0.75, 0.05))
	b.WriteString("scroll_container[0.35,2.1;6.9,6.1;scr_edit_students;vertical;0.1]")
	y := 0.05
	for _, s := range students {
		b.WriteString(box(0, y, 6.75, 0.65, panel))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.25, fmtEsc(s)))
		b.WriteString(fmt.Sprintf("button[5.55,%g;0.8,0.45;rm_student_%s;%s]", y+0.1, fmtEsc(s), fmtEsc(mcColorize(danger, "Rm"))))
		y += 0.75
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:students", b.String())
}
