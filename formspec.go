package main

import (
	"fmt"
	"sort"
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
	b.WriteString("size[12,10]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	// Header
	b.WriteString(box(0, 0, 12, 1.1, panel))
	b.WriteString(coloredLbl(0.3, 0.5, accent, "| "))
	b.WriteString(coloredLbl(0.7, 0.5, light, "Classrooms - Teacher Dashboard"))
	b.WriteString(box(0, 1.1, 12, 0.05, accent))

	// Create class section
	b.WriteString(box(0.3, 1.4, 11.4, 1.5, panel))
	b.WriteString(coloredLbl(0.6, 1.8, muted, "New class name:"))
	b.WriteString("field[3.5,1.7;5.5,0.8;new_class_name;;]")
	b.WriteString("field_close_on_enter[new_class_name;false]")
	b.WriteString(btn(9.2, 1.7, 2.2, 0.8, "btn_create_class", "+ Create"))

	// List header
	b.WriteString(coloredLbl(0.5, 3.5, muted, "YOUR CLASSES"))
	b.WriteString(box(0, 3.8, 12, 0.04, panel))

	if len(classes) == 0 {
		b.WriteString(coloredLbl(0.5, 4.5, muted, "No classes yet. Create one above to get started."))
	} else {
		b.WriteString("scroll_container[0,4;12,5;scr_classes;vertical;0.1]")
		y := 0.2
		for _, cls := range classes {
			students, _ := c.getStudents(cls.ID)
			online := 0
			for _, s := range students {
				if proxy.Find(s) != nil {
					online++
				}
			}

			b.WriteString(box(0.2, y, 11.6, 1.0, panel))
			b.WriteString(fmt.Sprintf("label[0.5,%g;%s]", y+0.4,
				fmtEsc(mcColorize(light, cls.Name)+
					mcColorize(muted, fmt.Sprintf("  (%d/%d online)", online, len(students))))))
			
			b.WriteString(fmt.Sprintf("button[8.0,%g;1.8,0.8;open_class_%d;Open]", y+0.1, cls.ID))
			b.WriteString(fmt.Sprintf("button[10.0,%g;1.8,0.8;del_class_%d;%s]", y+0.1, cls.ID, fmtEsc(mcColorize(danger, "Delete"))))
			y += 1.2
		}
		b.WriteString("scroll_container_end[]")
	}

	b.WriteString(btnExit(0.3, 9.1, 2, 0.7, "btn_close", "Close"))
	if c.isAdmin(name) {
		b.WriteString(btn(9.7, 9.1, 2, 0.7, "btn_admin_panel", "Admin Panel"))
	}

	cc.ShowFormspec("classrooms:main", b.String())
}

// ── Class View (Students + Instances) ───────────────────────────────────────

func (c *controller) showClassView(cc *proxy.ClientConn, classID int) {
	cls, err := c.getClassByID(classID)
	if err != nil || cls == nil {
		c.showMainDashboard(cc)
		return
	}
	c.setActiveClass(cc.Name(), classID)

	students, _ := c.getStudents(classID)
	instances, _ := c.getInstancesForClass(classID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[14,12]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	// Header
	b.WriteString(box(0, 0, 14, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.5, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(2.0, 0.5, accent, "| "))
	b.WriteString(coloredLbl(2.4, 0.5, light, "Class: "+cls.Name))
	b.WriteString(box(0, 1.1, 14, 0.05, accent))

	// Left Column: Student Controls (6 units wide)
	b.WriteString(coloredLbl(0.5, 1.5, muted, "STUDENTS"))
	b.WriteString(box(0.3, 1.8, 6.2, 9.2, panel))
	
	b.WriteString(btn(0.5, 2.0, 2.8, 0.7, "btn_freeze_all", "Freeze All"))
	b.WriteString(btn(3.5, 2.0, 2.8, 0.7, "btn_unfreeze_all", "Unfreeze All"))
	b.WriteString(btn(0.5, 2.8, 2.8, 0.7, "btn_watch_teacher", "Watch Me"))
	b.WriteString(btn(3.5, 2.8, 2.8, 0.7, "btn_stop_watching", "Stop Watch"))
	b.WriteString(btn(0.5, 3.6, 2.8, 0.7, "btn_gather_all", "Gather All"))
	b.WriteString(btn(3.5, 3.6, 2.8, 0.7, "btn_manage_students", "Edit Names"))

	b.WriteString("scroll_container[0.5,4.5;6.0,6.0;scr_students;vertical;0.1]")
	sy := 0.1
	for _, s := range students {
		online := proxy.Find(s) != nil
		statusColor := muted
		if online { statusColor = success }
		
		b.WriteString(box(0, sy, 5.5, 0.8, headerColor))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", sy+0.35, fmtEsc(mcColorize(statusColor, s))))
		
		if online {
			b.WriteString(fmt.Sprintf("button[3.2,%g;1.0,0.6;tp_to_%s;TP]", sy+0.1, fmtEsc(s)))
			b.WriteString(fmt.Sprintf("button[4.3,%g;1.0,0.6;watch_%s;Eye]", sy+0.1, fmtEsc(s)))
		}
		sy += 0.9
	}
	b.WriteString("scroll_container_end[]")

	// Right Column: Instances (7 units wide)
	b.WriteString(coloredLbl(7.0, 1.5, muted, "INSTANCES"))
	b.WriteString(box(6.8, 1.8, 6.9, 9.2, panel))
	b.WriteString(btn(7.0, 2.0, 6.5, 0.8, "btn_create_instance", "+ Provision New Instance"))

	b.WriteString("scroll_container[7.0,3.0;6.5,7.5;scr_instances;vertical;0.1]")
	iy := 0.1
	for _, inst := range instances {
		statusColor := muted
		switch inst.Status {
		case "running": statusColor = success
		case "provisioning": statusColor = warning
		case "stopped": statusColor = danger
		}

		b.WriteString(box(0, iy, 6.2, 1.2, headerColor))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.35, fmtEsc(mcColorize(light, inst.TemplateName))))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.75, fmtEsc(mcColorize(statusColor, inst.Status))))
		
		b.WriteString(fmt.Sprintf("button[4.0,%g;2.0,0.8;open_inst_%s;Manage]", iy+0.2, fmtEsc(inst.ID)))
		iy += 1.4
	}
	b.WriteString("scroll_container_end[]")

	b.WriteString(btnExit(0.3, 11.2, 2, 0.7, "btn_close", "Close"))
	cc.ShowFormspec("classrooms:class", b.String())
}

// ── Instance Creation (Template Picker) ─────────────────────────────────────

func (c *controller) showTemplatePicker(cc *proxy.ClientConn, classID *int) {
	templates := c.getPublicTemplates()
	if c.isAdmin(cc.Name()) {
		templates = c.getAllTemplates()
	}

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,8]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.5, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(2.0, 0.5, light, "Select Template"))
	b.WriteString(box(0, 1.1, 10, 0.05, accent))

	b.WriteString("scroll_container[0.5,1.5;9,6;scr_templates;vertical;0.1]")
	y := 0.2
	for _, tName := range templates {
		tpl := c.cfg.Templates[tName]
		b.WriteString(box(0, y, 8.5, 1.2, panel))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", y+0.4, fmtEsc(mcColorize(light, tName))))
		b.WriteString(fmt.Sprintf("label[0.3,%g;%s]", y+0.8, fmtEsc(mcColorize(muted, tpl.ServerDescription))))
		b.WriteString(fmt.Sprintf("button[6.5,%g;1.8,0.8;pick_tpl_%s;Select]", y+0.2, fmtEsc(tName)))
		y += 1.4
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:template_picker", b.String())
}

// ── Instance Detail View ────────────────────────────────────────────────────

func (c *controller) showInstanceView(cc *proxy.ClientConn, instanceID string) {
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil {
		c.showMainDashboard(cc)
		return
	}
	c.setActiveInstance(cc.Name(), instanceID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,10]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.5, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(2.0, 0.5, light, "Instance: "+inst.ID))
	b.WriteString(box(0, 1.1, 10, 0.05, accent))

	// Info Box
	b.WriteString(box(0.5, 1.5, 9, 3, panel))
	b.WriteString(coloredLbl(0.8, 2.0, muted, "Template: "+inst.TemplateName))
	b.WriteString(coloredLbl(0.8, 2.5, muted, "Creator:  "+inst.CreatedBy))
	
	statusColor := muted
	switch inst.Status {
	case "running": statusColor = success
	case "provisioning": statusColor = warning
	case "stopped": statusColor = danger
	}
	b.WriteString(fmt.Sprintf("label[0.8,3.0;%s]", fmtEsc(mcColorize(muted, "Status:   ")+mcColorize(statusColor, inst.Status))))
	b.WriteString(coloredLbl(0.8, 3.5, muted, "Address:  "+inst.ProxyName))

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

// ── Admin Panel ─────────────────────────────────────────────────────────────

func (c *controller) showAdminPanel(cc *proxy.ClientConn) {
	if !c.isAdmin(cc.Name()) {
		c.showMainDashboard(cc)
		return
	}

	instances, _ := c.getAllActiveInstances()
	teachers, _ := c.listTeachers()

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[14,10]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 14, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.5, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(2.0, 0.5, accent, "| "))
	b.WriteString(coloredLbl(2.4, 0.5, light, "Classrooms - GLOBAL ADMIN"))
	b.WriteString(box(0, 1.1, 14, 0.05, danger))

	// Instances list (Left)
	b.WriteString(coloredLbl(0.5, 1.5, muted, "ALL INSTANCES"))
	b.WriteString("scroll_container[0.3,1.8;8,7.5;scr_admin_inst;vertical;0.1]")
	iy := 0.1
	for _, inst := range instances {
		b.WriteString(box(0, iy, 7.5, 1.2, panel))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.4, fmtEsc(mcColorize(light, inst.ID))))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", iy+0.8, fmtEsc(mcColorize(muted, inst.CreatedBy+" | "+inst.Status))))
		b.WriteString(fmt.Sprintf("button[5.5,%g;1.8,0.8;open_inst_%s;Open]", iy+0.2, fmtEsc(inst.ID)))
		iy += 1.4
	}
	b.WriteString("scroll_container_end[]")

	// Teachers list (Right)
	b.WriteString(coloredLbl(8.8, 1.5, muted, "TEACHERS"))
	b.WriteString(box(8.5, 1.8, 5.2, 7.5, panel))
	
	b.WriteString("field[8.7,2.1;3.5,0.7;new_teacher_name;;]")
	b.WriteString(btn(12.3, 2.1, 1.2, 0.7, "btn_admin_add_teacher", "Add"))

	b.WriteString("scroll_container[8.7,3.0;4.8,6.0;scr_admin_teachers;vertical;0.1]")
	ty := 0.1
	sort.Strings(teachers)
	for _, t := range teachers {
		b.WriteString(fmt.Sprintf("label[0,%g;%s]", ty+0.3, fmtEsc(t)))
		b.WriteString(fmt.Sprintf("button[3.2,%g;1.2,0.6;rm_teacher_%s;%s]", ty, fmtEsc(t), fmtEsc(mcColorize(danger, "Rm"))))
		ty += 0.8
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:admin", b.String())
}

// ── Student Names List ──────────────────────────────────────────────────────

func (c *controller) showStudentEditor(cc *proxy.ClientConn, classID int) {
	cls, _ := c.getClassByID(classID)
	students, _ := c.getStudents(classID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[8,10]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 8, 1.1, panel))
	b.WriteString(btn(0.2, 0.2, 1.2, 0.7, "btn_back", "Back"))
	b.WriteString(coloredLbl(1.6, 0.5, light, "Edit Students: "+cls.Name))
	b.WriteString(box(0, 1.1, 8, 0.05, accent))

	b.WriteString("field[0.5,1.7;5.5,0.8;add_student_name;;]")
	b.WriteString(btn(6.2, 1.7, 1.3, 0.8, "btn_add_student", "Add"))

	b.WriteString("scroll_container[0.5,3.0;7,6;scr_edit_students;vertical;0.1]")
	y := 0.1
	for _, s := range students {
		b.WriteString(fmt.Sprintf("label[0,%g;%s]", y+0.35, fmtEsc(s)))
		b.WriteString(fmt.Sprintf("button[5.5,%g;1.2,0.7;rm_student_%s;%s]", y, fmtEsc(s), fmtEsc(mcColorize(danger, "Rm"))))
		y += 0.9
	}
	b.WriteString("scroll_container_end[]")

	cc.ShowFormspec("classrooms:students", b.String())
}
