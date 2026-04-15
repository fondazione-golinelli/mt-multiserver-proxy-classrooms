package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func (c *controller) registerHandlers() {
	proxy.RegisterOnPlayerReceiveFields("classrooms:main", c.handleMainDashboard)
	proxy.RegisterOnPlayerReceiveFields("classrooms:class", c.handleClassView)
	proxy.RegisterOnPlayerReceiveFields("classrooms:template_picker", c.handleTemplatePicker)
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance", c.handleInstanceView)
	proxy.RegisterOnPlayerReceiveFields("classrooms:admin", c.handleAdminPanel)
	proxy.RegisterOnPlayerReceiveFields("classrooms:students", c.handleStudentEditor)
}

func fieldMap(fields []mt.Field) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[f.Name] = f.Value
	}
	return m
}

func (c *controller) notify(cc *proxy.ClientConn, msg string) {
	cc.SendChatMsg("[Classrooms] " + msg)
}

// ── Main Dashboard Handler ──────────────────────────────────────────────────

func (c *controller) handleMainDashboard(cc *proxy.ClientConn, fields []mt.Field) {
	name := cc.Name()
	if !c.isTeacher(name) {
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_create_class"]; ok {
		className := strings.TrimSpace(fm["new_class_name"])
		if ok, msg := c.createClass(name, className); !ok {
			c.notify(cc, msg)
		}
		c.showMainDashboard(cc)
		return
	}

	if _, ok := fm["btn_admin_panel"]; ok {
		c.showAdminPanel(cc)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_class_") {
			idStr := strings.TrimPrefix(k, "open_class_")
			id, _ := strconv.Atoi(idStr)
			c.showClassView(cc, id)
			return
		}
		if strings.HasPrefix(k, "del_class_") {
			idStr := strings.TrimPrefix(k, "del_class_")
			id, _ := strconv.Atoi(idStr)
			if ok, msg := c.deleteClass(name, id); !ok {
				c.notify(cc, msg)
			}
			c.showMainDashboard(cc)
			return
		}
	}
}

// ── Class View Handler ──────────────────────────────────────────────────────

func (c *controller) handleClassView(cc *proxy.ClientConn, fields []mt.Field) {
	classID, ok := c.getActiveClass(cc.Name())
	if !ok {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		c.showMainDashboard(cc)
		return
	}

	if _, ok := fm["btn_manage_students"]; ok {
		c.showStudentEditor(cc, classID)
		return
	}

	if _, ok := fm["btn_create_instance"]; ok {
		c.showTemplatePicker(cc, &classID)
		return
	}

	if _, ok := fm["btn_freeze_all"]; ok {
		c.freezeClass(classID)
		c.showClassView(cc, classID)
		return
	}
	if _, ok := fm["btn_unfreeze_all"]; ok {
		c.unfreezeClass(classID)
		c.showClassView(cc, classID)
		return
	}
	if _, ok := fm["btn_watch_teacher"]; ok {
		c.watchTeacher(classID, cc.Name())
		c.showClassView(cc, classID)
		return
	}
	if _, ok := fm["btn_stop_watching"]; ok {
		c.stopWatching(classID)
		c.showClassView(cc, classID)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_inst_") {
			instID := strings.TrimPrefix(k, "open_inst_")
			c.showInstanceView(cc, instID)
			return
		}
		if strings.HasPrefix(k, "tp_to_") {
			target := strings.TrimPrefix(k, "tp_to_")
			cc.SendModChanMsg(modChannel, fmt.Sprintf(`{"action":"tp_to","player":"%s","target":"%s"}`, cc.Name(), target))
			c.showClassView(cc, classID)
			return
		}
		if strings.HasPrefix(k, "watch_") {
			target := strings.TrimPrefix(k, "watch_")
			cc.SendModChanMsg(modChannel, fmt.Sprintf(`{"action":"watch","player":"%s","teacher":"%s"}`, target, cc.Name()))
			c.showClassView(cc, classID)
			return
		}
	}
}

// ── Template Picker Handler ─────────────────────────────────────────────────

func (c *controller) handleTemplatePicker(cc *proxy.ClientConn, fields []mt.Field) {
	classID, hasClass := c.getActiveClass(cc.Name())
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		if hasClass {
			c.showClassView(cc, classID)
		} else {
			c.showAdminPanel(cc)
		}
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "pick_tpl_") {
			tName := strings.TrimPrefix(k, "pick_tpl_")
			c.notify(cc, "Provisioning instance from template "+tName+"...")
			
			var pClassID *int
			if hasClass { pClassID = &classID }
			
			go func() {
				inst, err := c.provisionInstance(pClassID, cc.Name(), tName)
				if err != nil {
					c.notify(cc, "Provisioning failed: "+err.Error())
					return
				}
				c.notify(cc, "Instance "+inst.ProxyName+" is ready.")
			}()
			
			if hasClass {
				c.showClassView(cc, classID)
			} else {
				c.showAdminPanel(cc)
			}
			return
		}
	}
}

// ── Instance View Handler ───────────────────────────────────────────────────

func (c *controller) handleInstanceView(cc *proxy.ClientConn, fields []mt.Field) {
	instID, ok := c.getActiveInstance(cc.Name())
	if !ok {
		c.showMainDashboard(cc)
		return
	}
	inst, _ := c.getInstanceByID(instID)
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		if inst != nil && inst.ClassID != nil {
			c.showClassView(cc, *inst.ClassID)
		} else {
			c.showAdminPanel(cc)
		}
		return
	}

	if _, ok := fm["btn_inst_start"]; ok && inst != nil {
		c.notify(cc, "Starting instance...")
		go func() {
			if err := c.startInstance(inst); err != nil {
				c.notify(cc, "Start failed: "+err.Error())
			}
		}()
		c.showInstanceView(cc, instID)
		return
	}

	if _, ok := fm["btn_inst_stop"]; ok && inst != nil {
		c.notify(cc, "Stopping instance...")
		go func() {
			if err := c.stopInstance(inst); err != nil {
				c.notify(cc, "Stop failed: "+err.Error())
			}
		}()
		c.showInstanceView(cc, instID)
		return
	}

	if _, ok := fm["btn_inst_delete"]; ok && inst != nil {
		c.notify(cc, "Deleting instance...")
		go func() {
			if err := c.deleteInstance(inst); err != nil {
				c.notify(cc, "Delete failed: "+err.Error())
			}
		}()
		if inst.ClassID != nil {
			c.showClassView(cc, *inst.ClassID)
		} else {
			c.showAdminPanel(cc)
		}
		return
	}

	if _, ok := fm["btn_hop_me"]; ok && inst != nil {
		cc.Hop(inst.ProxyName)
		return
	}

	if _, ok := fm["btn_hop_class"]; ok && inst != nil && inst.ClassID != nil {
		students := c.getOnlineStudents(*inst.ClassID)
		for _, s := range students {
			if scc := proxy.Find(s); scc != nil {
				scc.Hop(inst.ProxyName)
			}
		}
		return
	}

	if _, ok := fm["btn_invite"]; ok {
		invitee := strings.TrimSpace(fm["invite_name"])
		if invitee != "" {
			c.addInstanceInvite(instID, invitee)
			c.notify(cc, "Invited "+invitee)
		}
		c.showInstanceView(cc, instID)
		return
	}
}

// ── Admin Panel Handler ─────────────────────────────────────────────────────

func (c *controller) handleAdminPanel(cc *proxy.ClientConn, fields []mt.Field) {
	if !c.isAdmin(cc.Name()) {
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		c.showMainDashboard(cc)
		return
	}

	if _, ok := fm["btn_admin_add_teacher"]; ok {
		tName := strings.TrimSpace(fm["new_teacher_name"])
		if tName != "" {
			c.addTeacher(tName)
		}
		c.showAdminPanel(cc)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_inst_") {
			instID := strings.TrimPrefix(k, "open_inst_")
			c.showInstanceView(cc, instID)
			return
		}
		if strings.HasPrefix(k, "rm_teacher_") {
			tName := strings.TrimPrefix(k, "rm_teacher_")
			c.removeTeacher(tName)
			c.showAdminPanel(cc)
			return
		}
	}
}

// ── Student Editor Handler ──────────────────────────────────────────────────

func (c *controller) handleStudentEditor(cc *proxy.ClientConn, fields []mt.Field) {
	classID, ok := c.getActiveClass(cc.Name())
	if !ok {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		c.showClassView(cc, classID)
		return
	}

	if _, ok := fm["btn_add_student"]; ok {
		sName := strings.TrimSpace(fm["add_student_name"])
		if ok, msg := c.addStudent(classID, sName); !ok {
			c.notify(cc, msg)
		}
		c.showStudentEditor(cc, classID)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "rm_student_") {
			sName := strings.TrimPrefix(k, "rm_student_")
			c.removeStudent(classID, sName)
			c.showStudentEditor(cc, classID)
			return
		}
	}
}
