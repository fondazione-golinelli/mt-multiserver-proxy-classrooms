package main

import (
	"log"
	"strconv"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func (c *controller) registerHandlers() {
	proxy.RegisterOnPlayerReceiveFields("classrooms:main", c.handleMainDashboard)
	proxy.RegisterOnPlayerReceiveFields("classrooms:class", c.handleClassView)
	proxy.RegisterOnPlayerReceiveFields("classrooms:template_picker", c.handleTemplatePicker)
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance_progress", c.handleInstanceProgress)
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance_ready", c.handleInstanceReady)
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance_error", c.handleInstanceError)
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

func cleanInstanceDisplayName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")
	name = strings.ReplaceAll(name, ";", " ")
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}

func activeClassPtr(hasClass bool, classID int) *int {
	if !hasClass {
		return nil
	}
	id := classID
	return &id
}

func (c *controller) showParentForInstance(cc *proxy.ClientConn, inst *instanceData) {
	switch c.getActiveInstanceOrigin(cc.Name()) {
	case viewOriginAdminInstances:
		c.showAdminPanelTab(cc, "instances")
		return
	case viewOriginAdminClasses:
		if inst != nil && inst.ClassID != nil {
			c.showClassViewWithOrigin(cc, *inst.ClassID, viewOriginAdminClasses)
			return
		}
		if classID, ok := c.getActiveClass(cc.Name()); ok {
			c.showClassViewWithOrigin(cc, classID, viewOriginAdminClasses)
			return
		}
		c.showAdminPanelTab(cc, "classes")
		return
	}
	if inst != nil && inst.ClassID != nil {
		c.showClassView(cc, *inst.ClassID)
		return
	}
	c.showAdminPanel(cc)
}

func (c *controller) hopClassToInstance(cc *proxy.ClientConn, inst *instanceData, includeTeacher bool) {
	if inst == nil || inst.ClassID == nil {
		return
	}
	if includeTeacher && cc.ServerName() != inst.ProxyName {
		_ = cc.Hop(inst.ProxyName)
	}
	students := c.getOnlineStudents(*inst.ClassID)
	for _, s := range students {
		if scc := proxy.Find(s); scc != nil && scc.ServerName() != inst.ProxyName {
			_ = scc.Hop(inst.ProxyName)
		}
	}
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
	log.Printf("[%s] handleClassView: user=%s, fields=%v", pluginName, cc.Name(), fields)
	classID, ok := c.getActiveClass(cc.Name())
	if !ok {
		log.Printf("[%s] handleClassView: no active class for %s", pluginName, cc.Name())
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		if c.getActiveClassOrigin(cc.Name()) == viewOriginAdminClasses {
			c.showAdminPanelTab(cc, "classes")
		} else {
			c.showMainDashboard(cc)
		}
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

	if _, ok := fm["btn_gather_all"]; ok {
		c.gatherClass(classID, cc.Name())
		c.showClassView(cc, classID)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_inst_") {
			instID := strings.TrimPrefix(k, "open_inst_")
			if c.getActiveClassOrigin(cc.Name()) == viewOriginAdminClasses {
				c.showInstanceViewWithOrigin(cc, instID, viewOriginAdminClasses)
			} else {
				c.showInstanceView(cc, instID)
			}
			return
		}
		if strings.HasPrefix(k, "tp_to_") {
			target := strings.TrimPrefix(k, "tp_to_")
			c.teleportToPlayer(cc, target)
			c.showClassView(cc, classID)
			return
		}
		if strings.HasPrefix(k, "watch_") {
			target := strings.TrimPrefix(k, "watch_")
			c.watchSingleStudent(cc.Name(), target)
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
			c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
		} else {
			c.showAdminPanelTab(cc, c.getAdminTab(cc.Name()))
		}
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "pick_tpl_") {
			tName := strings.TrimPrefix(k, "pick_tpl_")
			displayName := cleanInstanceDisplayName(fm["new_instance_name"])
			if displayName == "" {
				c.notify(cc, "Instance name is required.")
				c.showTemplatePicker(cc, activeClassPtr(hasClass, classID))
				return
			}
			player := cc.Name()
			if !c.beginOp(player) {
				c.notify(cc, "Another server operation is already running. Please wait for it to finish.")
				return
			}

			var pClassID *int
			if hasClass {
				classIDCopy := classID
				pClassID = &classIDCopy
			}

			c.showInstanceProgress(cc, "Creating server", "Provisioning "+displayName+".")

			go func() {
				defer c.endOp(player)

				inst, err := c.provisionInstance(pClassID, player, tName, displayName)
				liveCC := proxy.Find(player)
				if liveCC == nil {
					return
				}
				if err != nil {
					c.showInstanceError(liveCC, inst, "Server creation failed", err.Error())
					return
				}
				c.showInstanceReady(liveCC, inst, "Server ready")
			}()

			return
		}
	}
}

// ── Instance Operation Dialog Handlers ──────────────────────────────────────

func (c *controller) handleInstanceProgress(cc *proxy.ClientConn, fields []mt.Field) {
	fm := fieldMap(fields)
	if _, ok := fm["btn_progress_close"]; ok {
		return
	}
}

func (c *controller) handleInstanceReady(cc *proxy.ClientConn, fields []mt.Field) {
	instID, ok := c.getActiveInstance(cc.Name())
	if !ok {
		c.showMainDashboard(cc)
		return
	}
	inst, _ := c.getInstanceByID(instID)
	if inst == nil {
		c.showMainDashboard(cc)
		return
	}

	fm := fieldMap(fields)
	if _, ok := fm["btn_ready_hop_me"]; ok {
		if cc.ServerName() != inst.ProxyName {
			_ = cc.Hop(inst.ProxyName)
		}
		return
	}
	if _, ok := fm["btn_ready_hop_class"]; ok {
		c.hopClassToInstance(cc, inst, true)
		return
	}
	if _, ok := fm["btn_ready_open"]; ok {
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	if _, ok := fm["btn_ready_close"]; ok {
		return
	}
}

func (c *controller) handleInstanceError(cc *proxy.ClientConn, fields []mt.Field) {
	fm := fieldMap(fields)
	if _, ok := fm["btn_error_open"]; ok {
		if instID, ok := c.getActiveInstance(cc.Name()); ok {
			c.showInstanceViewWithOrigin(cc, instID, c.getActiveInstanceOrigin(cc.Name()))
			return
		}
	}
	if _, ok := fm["btn_error_close"]; ok {
		return
	}
}

// ── Instance View Handler ───────────────────────────────────────────────────

func (c *controller) handleInstanceView(cc *proxy.ClientConn, fields []mt.Field) {
	instID, ok := c.getActiveInstance(cc.Name())
	if !ok {
		c.showMainDashboard(cc)
		return
	}
	origin := c.getActiveInstanceOrigin(cc.Name())
	inst, _ := c.getInstanceByID(instID)
	fm := fieldMap(fields)

	if _, ok := fm["btn_back"]; ok {
		c.showParentForInstance(cc, inst)
		return
	}

	if _, ok := fm["btn_inst_start"]; ok && inst != nil {
		player := cc.Name()
		if !c.beginOp(player) {
			c.notify(cc, "Another server operation is already running. Please wait for it to finish.")
			return
		}

		c.showInstanceProgress(cc, "Starting server", "Starting "+inst.ProxyName+".")
		go func() {
			defer c.endOp(player)

			if err := c.startInstance(inst); err != nil {
				if liveCC := proxy.Find(player); liveCC != nil {
					c.showInstanceError(liveCC, inst, "Server start failed", err.Error())
				}
				return
			}
			liveCC := proxy.Find(player)
			if liveCC == nil {
				return
			}
			updated, err := c.getInstanceByID(inst.ID)
			if err == nil && updated != nil {
				inst = updated
			}
			c.showInstanceReady(liveCC, inst, "Server ready")
		}()
		return
	}

	if _, ok := fm["btn_inst_stop"]; ok && inst != nil {
		c.notify(cc, "Stopping instance...")
		go func() {
			if err := c.stopInstance(inst); err != nil {
				c.notify(cc, "Stop failed: "+err.Error())
			}
		}()
		c.showInstanceViewWithOrigin(cc, instID, origin)
		return
	}

	if _, ok := fm["btn_inst_delete"]; ok && inst != nil {
		c.notify(cc, "Deleting instance...")
		go func() {
			if err := c.deleteInstance(inst); err != nil {
				c.notify(cc, "Delete failed: "+err.Error())
			}
		}()
		c.showInstanceFallback(cc, origin)
		return
	}

	if _, ok := fm["btn_hop_me"]; ok && inst != nil {
		if cc.ServerName() != inst.ProxyName {
			cc.Hop(inst.ProxyName)
		}
		return
	}

	if _, ok := fm["btn_hop_class"]; ok && inst != nil && inst.ClassID != nil {
		c.hopClassToInstance(cc, inst, true)
		return
	}

	if _, ok := fm["btn_invite"]; ok {
		invitee := strings.TrimSpace(fm["invite_name"])
		if invitee != "" {
			c.addInstanceInvite(instID, invitee)
			c.notify(cc, "Invited "+invitee)
		}
		c.showInstanceViewWithOrigin(cc, instID, origin)
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
		return
	}
	if _, ok := fm["btn_close"]; ok {
		return
	}

	if _, ok := fm["btn_admin_tab_instances"]; ok {
		c.showAdminPanelTab(cc, "instances")
		return
	}

	if _, ok := fm["btn_admin_tab_classes"]; ok {
		c.showAdminPanelTab(cc, "classes")
		return
	}

	if _, ok := fm["btn_admin_tab_teachers"]; ok {
		c.showAdminPanelTab(cc, "teachers")
		return
	}

	if _, ok := fm["btn_admin_filter_apply"]; ok {
		institute := strings.TrimSpace(fm["admin_filter_institute"])
		teacher := strings.TrimSpace(fm["admin_filter_teacher"])
		c.setAdminFilters(cc.Name(), institute, teacher)
		c.showAdminPanelTab(cc, c.getAdminTab(cc.Name()))
		return
	}

	if _, ok := fm["btn_admin_filter_clear"]; ok {
		c.setAdminFilters(cc.Name(), "", "")
		c.showAdminPanelTab(cc, c.getAdminTab(cc.Name()))
		return
	}

	if _, ok := fm["btn_admin_add_teacher"]; ok {
		tName := strings.TrimSpace(fm["new_teacher_name"])
		if tName != "" {
			institute := strings.TrimSpace(fm["new_teacher_institute"])
			c.addTeacherWithInstitute(tName, institute)
		}
		c.showAdminPanelTab(cc, "teachers")
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_inst_") {
			instID := strings.TrimPrefix(k, "open_inst_")
			c.showInstanceViewWithOrigin(cc, instID, viewOriginAdminInstances)
			return
		}
		if strings.HasPrefix(k, "open_class_") {
			idStr := strings.TrimPrefix(k, "open_class_")
			id, _ := strconv.Atoi(idStr)
			c.showClassViewWithOrigin(cc, id, viewOriginAdminClasses)
			return
		}
		if strings.HasPrefix(k, "del_class_") {
			idStr := strings.TrimPrefix(k, "del_class_")
			id, _ := strconv.Atoi(idStr)
			if ok, msg := c.deleteClass(cc.Name(), id); !ok {
				c.notify(cc, msg)
			}
			c.showAdminPanelTab(cc, "classes")
			return
		}
		if strings.HasPrefix(k, "rm_teacher_") {
			tName := strings.TrimPrefix(k, "rm_teacher_")
			c.removeTeacher(tName)
			c.showAdminPanelTab(cc, "teachers")
			return
		}
		if strings.HasPrefix(k, "save_teacher_") {
			tName := strings.TrimPrefix(k, "save_teacher_")
			fieldName := "teacher_institute_" + tName
			institute := strings.TrimSpace(fm[fieldName])
			c.updateTeacherInstitute(tName, institute)
			c.showAdminPanelTab(cc, "teachers")
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
		c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
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
