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
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance_settings", c.handleInstanceSettings)
	proxy.RegisterOnPlayerReceiveFields("classrooms:instance_restart", c.handleInstanceRestart)
	proxy.RegisterOnPlayerReceiveFields("classrooms:world_controls", c.handleWorldControls)
	proxy.RegisterOnPlayerReceiveFields("classrooms:admin", c.handleAdminPanel)
	proxy.RegisterOnPlayerReceiveFields("classrooms:students", c.handleStudentEditor)
	proxy.RegisterOnPlayerReceiveFields("classrooms:assistants", c.handleAssistanceEditor)
	proxy.RegisterOnPlayerReceiveFields("classrooms:teachers", c.handleClassTeacherEditor)
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
	recipients := make(map[string]struct{})
	if includeTeacher && cc != nil {
		recipients[cc.Name()] = struct{}{}
	}
	students := c.getOnlineStudents(*inst.ClassID)
	for _, name := range students {
		recipients[name] = struct{}{}
	}
	staff, err := c.getClassStaff(*inst.ClassID)
	if err != nil {
		log.Printf("[%s] could not load class staff for class %d: %v", pluginName, *inst.ClassID, err)
	}
	for _, name := range staff {
		recipients[name] = struct{}{}
	}
	for name := range recipients {
		if player := proxy.Find(name); player != nil && player.ServerName() != inst.ProxyName {
			_ = c.hopPlayer(player, inst.ProxyName)
		}
	}
}

// ── Main Dashboard Handler ──────────────────────────────────────────────────

func (c *controller) handleMainDashboard(cc *proxy.ClientConn, fields []mt.Field) {
	name := cc.Name()
	if !c.hasClassPanelAccess(name) {
		return
	}
	fm := fieldMap(fields)

	if _, ok := fm["btn_create_class"]; ok {
		if !c.isTeacher(name) {
			return
		}
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
			if c.canViewClass(id, name) {
				c.showClassView(cc, id)
			}
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
	canManage := c.canManageClass(classID, cc.Name())
	canEditStudents := c.canEditClassStudents(classID, cc.Name())
	if !canEditStudents {
		c.showMainDashboard(cc)
		return
	}

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
	if _, ok := fm["btn_manage_assistance"]; ok {
		if canManage {
			c.showClassMemberEditor(cc, classID, true)
		}
		return
	}
	if _, ok := fm["btn_manage_teachers"]; ok {
		if canManage {
			c.showClassMemberEditor(cc, classID, false)
		}
		return
	}

	// Assistance may edit students, TP to them, and join a running world that
	// belongs to the active class.
	if !canManage {
		for k := range fm {
			if strings.HasPrefix(k, "tp_to_") {
				target := strings.TrimPrefix(k, "tp_to_")
				if c.isStudentInClass(classID, target) {
					c.teleportToPlayer(cc, target)
				}
				c.showClassView(cc, classID)
				return
			}
			if strings.HasPrefix(k, "join_inst_") {
				instanceID := strings.TrimPrefix(k, "join_inst_")
				inst, err := c.getInstanceByID(instanceID)
				if err == nil && inst != nil && inst.Status == "running" && inst.ClassID != nil && *inst.ClassID == classID {
					if cc.ServerName() != inst.ProxyName {
						if err := c.hopPlayer(cc, inst.ProxyName); err != nil {
							c.notify(cc, "Could not join the class world: "+err.Error())
						}
					}
				} else {
					c.notify(cc, "That class world is no longer available.")
				}
				return
			}
		}
		return
	}

	if _, ok := fm["btn_create_instance"]; ok {
		c.showTemplatePicker(cc, &classID)
		return
	}

	if _, ok := fm["btn_toggle_freeze"]; ok {
		if c.isClassFrozen(classID) {
			c.unfreezeClass(classID)
		} else {
			c.freezeClass(classID)
		}
		c.showClassView(cc, classID)
		return
	}
	if _, ok := fm["btn_toggle_watch"]; ok {
		if c.isClassWatching(classID, cc.Name()) {
			c.stopWatching(classID)
		} else {
			c.watchTeacher(classID, cc.Name())
		}
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
			if c.isStudentInClass(classID, target) {
				c.teleportToPlayer(cc, target)
			}
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
	if !c.isTeacher(cc.Name()) || (hasClass && !c.canManageClass(classID, cc.Name())) {
		c.showMainDashboard(cc)
		return
	}
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
	if !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}

	fm := fieldMap(fields)
	if _, ok := fm["btn_ready_hop_me"]; ok {
		if cc.ServerName() != inst.ProxyName {
			_ = c.hopPlayer(cc, inst.ProxyName)
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
	if !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
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

	if _, ok := fm["btn_inst_settings"]; ok && inst != nil {
		c.showInstanceSettings(cc, inst.ID)
		return
	}

	if _, ok := fm["btn_world_controls"]; ok && inst != nil {
		if !c.instanceIsClassWorld(inst) {
			c.notify(cc, "World controls are only available for classroom instances.")
			c.showInstanceViewWithOrigin(cc, instID, origin)
			return
		}
		if cc.ServerName() != inst.ProxyName {
			c.notify(cc, "Hop to this instance server before using world controls.")
			c.showInstanceViewWithOrigin(cc, instID, origin)
			return
		}
		c.showWorldControls(cc, inst.ID)
		return
	}

	if _, ok := fm["btn_hop_me"]; ok && inst != nil {
		if cc.ServerName() != inst.ProxyName {
			_ = c.hopPlayer(cc, inst.ProxyName)
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

// ── Instance Settings Handler ──────────────────────────────────────────────

func (c *controller) handleInstanceSettings(cc *proxy.ClientConn, fields []mt.Field) {
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
	if !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)

	settingChanged := false
	settings, err := c.getInstanceSettingsOrDefault(inst.ID)
	if err != nil {
		c.notify(cc, "Could not load settings: "+err.Error())
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	if v, ok := fm["setting_damage"]; ok {
		settings.EnableDamage = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_pvp"]; ok {
		settings.EnablePVP = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_hunger"]; ok {
		settings.EnableHunger = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_mobs"]; ok {
		settings.MobsSpawn = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_peaceful"]; ok {
		settings.OnlyPeacefulMobs = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_explosions"]; ok {
		settings.ExplosionsGriefing = boolField(v)
		settingChanged = true
	}

	if _, ok := fm["btn_back"]; ok {
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	if _, ok := fm["btn_capture_spawn"]; ok {
		if cc.ServerName() != inst.ProxyName {
			c.notify(cc, "Hop to this instance before saving your position as spawn.")
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		if !cc.IsModChanJoined(modChannel) {
			go c.ensureChannelJoin(cc)
			c.notify(cc, "Instance control channel is still connecting. Try again in a moment.")
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		requestID := randomSuffix(8)
		c.mu.Lock()
		c.runtime.spawnCaptures[requestID] = spawnCaptureRequest{
			InstanceID: inst.ID,
			Teacher:    cc.Name(),
		}
		c.mu.Unlock()
		c.sendToPlayerServer(cc.Name(), map[string]string{
			"action":     "capture_spawnpoint",
			"player":     cc.Name(),
			"request_id": requestID,
		})
		c.notify(cc, "Asked the server to capture your current position.")
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if _, ok := fm["btn_save_settings"]; ok {
		if err := c.saveInstanceSettings(settings); err != nil {
			c.notify(cc, "Could not save settings: "+err.Error())
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		if c.sendSettingsToInstance(inst, settings) {
			c.notify(cc, "Settings saved. Damage and PvP apply immediately; mob, hunger, explosion, and spawnpoint changes need restart.")
		} else {
			c.notify(cc, "Settings saved. Start or join the instance before applying them to luanti.conf.")
		}
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if settingChanged {
		if err := c.saveInstanceSettings(settings); err != nil {
			c.notify(cc, "Could not save setting: "+err.Error())
		}
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if _, ok := fm["btn_restart_instance"]; ok {
		c.showInstanceRestartConfirm(cc, inst.ID)
		return
	}
}

func (c *controller) handleWorldControls(cc *proxy.ClientConn, fields []mt.Field) {
	instID, hasInstance := c.getActiveInstance(cc.Name())
	inst, _ := c.getInstanceByID(instID)
	if !hasInstance || !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["btn_back"]; ok {
		if instID, ok := c.getActiveInstance(cc.Name()); ok {
			c.showInstanceViewWithOrigin(cc, instID, c.getActiveInstanceOrigin(cc.Name()))
		} else if classID, ok := c.getActiveClass(cc.Name()); ok {
			c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
		} else {
			c.showMainDashboard(cc)
		}
		return
	}

	if _, ok := fm["btn_world_day"]; ok {
		c.applyWorldControls(cc, "day", "", false)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_evening"]; ok {
		c.applyWorldControls(cc, "evening", "", false)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_night"]; ok {
		c.applyWorldControls(cc, "night", "", false)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_stop_time"]; ok {
		c.applyWorldControls(cc, "stop", "", true)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_resume_time"]; ok {
		c.applyWorldControls(cc, "", "", false)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_sunny"]; ok {
		c.applyWorldControls(cc, "", "sunny", false)
		c.showWorldControls(cc, "")
		return
	}
	if _, ok := fm["btn_world_rain"]; ok {
		c.applyWorldControls(cc, "", "rain", false)
		c.showWorldControls(cc, "")
		return
	}
}

func (c *controller) handleInstanceRestart(cc *proxy.ClientConn, fields []mt.Field) {
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
	if !c.canManageInstance(inst, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["btn_back"]; ok {
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if _, ok := fm["btn_confirm_restart"]; ok {
		player := cc.Name()
		if !c.beginOp(player) {
			c.notify(cc, "Another server operation is already running. Please wait for it to finish.")
			return
		}
		displaced := c.playersOnInstance(inst.ProxyName)
		c.showInstanceProgress(cc, "Applying settings", "Moving players to lobby, restarting, then returning them.")
		go func() {
			defer c.endOp(player)
			if _, err := c.guidedRestartInstance(inst); err != nil {
				if liveCC := proxy.Find(player); liveCC != nil {
					c.showInstanceError(liveCC, inst, "Restart failed", err.Error())
				}
				return
			}
			liveCC := proxy.Find(player)
			if liveCC == nil {
				return
			}
			c.notify(liveCC, "Restart complete. Returned "+strconv.Itoa(len(displaced))+" displaced players.")
			updated, err := c.getInstanceByID(inst.ID)
			if err == nil && updated != nil {
				inst = updated
			}
			c.showInstanceReady(liveCC, inst, "Settings applied")
		}()
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
			if c.addTeacherWithInstitute(tName, institute) == nil && proxy.Find(tName) != nil {
				c.scheduleReapplyStates(tName, 0)
			}
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
			if c.removeTeacher(tName) == nil && proxy.Find(tName) != nil {
				c.scheduleReapplyStates(tName, 0)
			}
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
	if !c.canEditClassStudents(classID, cc.Name()) {
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

func (c *controller) handleAssistanceEditor(cc *proxy.ClientConn, fields []mt.Field) {
	c.handleClassMemberEditor(cc, fields, true)
}

func (c *controller) handleClassTeacherEditor(cc *proxy.ClientConn, fields []mt.Field) {
	c.handleClassMemberEditor(cc, fields, false)
}

func (c *controller) handleClassMemberEditor(cc *proxy.ClientConn, fields []mt.Field, assistance bool) {
	classID, ok := c.getActiveClass(cc.Name())
	if !ok || !c.canManageClass(classID, cc.Name()) {
		c.showMainDashboard(cc)
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["btn_back"]; ok {
		c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
		return
	}

	if assistance {
		if _, ok := fm["btn_add_assistance"]; ok {
			name := strings.TrimSpace(fm["add_assistance_name"])
			if ok, msg := c.addClassAssistant(classID, name); !ok {
				c.notify(cc, msg)
			}
			if proxy.Find(name) != nil {
				c.scheduleReapplyStates(name, 0)
			}
			c.showClassMemberEditor(cc, classID, true)
			return
		}
		for k := range fm {
			if strings.HasPrefix(k, "rm_assistance_") {
				name := strings.TrimPrefix(k, "rm_assistance_")
				c.removeClassAssistant(classID, name)
				if proxy.Find(name) != nil {
					c.scheduleReapplyStates(name, 0)
				}
				c.showClassMemberEditor(cc, classID, true)
				return
			}
		}
		return
	}

	if _, ok := fm["btn_add_teacher"]; ok {
		name := strings.TrimSpace(fm["add_teacher_name"])
		if ok, msg := c.addClassTeacher(classID, name); !ok {
			c.notify(cc, msg)
		}
		if proxy.Find(name) != nil {
			c.scheduleReapplyStates(name, 0)
		}
		c.showClassMemberEditor(cc, classID, false)
		return
	}
	for k := range fm {
		if strings.HasPrefix(k, "rm_teacher_") {
			name := strings.TrimPrefix(k, "rm_teacher_")
			c.removeClassTeacher(classID, name)
			if proxy.Find(name) != nil {
				c.scheduleReapplyStates(name, 0)
			}
			c.showClassMemberEditor(cc, classID, false)
			return
		}
	}
}
