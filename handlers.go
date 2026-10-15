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
	proxy.RegisterOnPlayerReceiveFields("classrooms:portal_worlds", c.handlePortalWorlds)
	proxy.RegisterOnPlayerReceiveFields("classrooms:groups", c.handleGroupsEditor)
	proxy.RegisterOnPlayerReceiveFields("classrooms:world_tools", c.handleWorldTools)
	proxy.RegisterOnPlayerReceiveFields("classrooms:mission", c.handleMissionEditor)
	proxy.RegisterOnPlayerReceiveFields("classrooms:password", c.handlePasswordDialog)
	proxy.RegisterOnPlayerReceiveFields("classrooms:admin_world", c.handleAdminWorldVisits)
	proxy.RegisterOnPlayerReceiveFields("classrooms:return_hub", c.handleReturnHubDialog)
	proxy.RegisterOnPlayerReceiveFields(minigamesForm, c.handleMinigames)
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

func hasPrefixKey(fm map[string]string, prefix string) bool {
	for k := range fm {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// prefixKey returns the first field whose name starts with prefix.
func prefixKey(fm map[string]string, prefix string) (string, bool) {
	for k := range fm {
		if strings.HasPrefix(k, prefix) {
			return k, true
		}
	}
	return "", false
}

// handleInstanceTabs switches between the Overview, Rules and Time & weather
// tabs of a world. It returns true when a tab button was pressed.
func (c *controller) handleInstanceTabs(cc *proxy.ClientConn, fm map[string]string, inst *instanceData) bool {
	if inst == nil {
		return false
	}
	if _, ok := fm[instTabOverview]; ok {
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return true
	}
	if _, ok := fm[instTabRules]; ok {
		c.showInstanceSettings(cc, inst.ID)
		return true
	}
	if _, ok := fm[instTabWorld]; ok {
		c.showWorldControls(cc, inst.ID)
		return true
	}

	return false
}

// handlePeopleTabs switches between the Students, Assistance and Teachers
// tabs of the people editor. It returns true when a tab button was pressed.
func (c *controller) handlePeopleTabs(cc *proxy.ClientConn, fm map[string]string, classID int) bool {
	if _, ok := fm[peopleTabStudents]; ok {
		c.showStudentEditor(cc, classID)
		return true
	}
	canManage := c.canManageClass(classID, cc.Name())
	if _, ok := fm[peopleTabGroups]; ok {
		if canManage {
			c.showGroupsEditor(cc, classID)
		}
		return true
	}
	if _, ok := fm[peopleTabAssistance]; ok {
		if canManage {
			c.showClassMemberEditor(cc, classID, true)
		}
		return true
	}
	if _, ok := fm[peopleTabTeachers]; ok {
		if canManage {
			c.showClassMemberEditor(cc, classID, false)
		}
		return true
	}
	return false
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

// studentsInClassWorlds are the online students of a class who are on one of
// its worlds.
func (c *controller) studentsInClassWorlds(classID int) []string {
	worlds, err := c.getInstancesForClass(classID)
	if err != nil {
		return nil
	}
	onWorld := make(map[string]bool, len(worlds))
	for _, w := range worlds {
		onWorld[w.ProxyName] = true
	}
	var result []string
	for _, name := range c.getOnlineStudents(classID) {
		if p := proxy.Find(name); p != nil && onWorld[p.ServerName()] {
			result = append(result, name)
		}
	}
	return result
}

func (c *controller) handleReturnHubDialog(cc *proxy.ClientConn, fields []mt.Field) {
	classID, ok := c.getActiveClass(cc.Name())
	if !ok || !c.canManageClass(classID, cc.Name()) {
		return
	}
	fm := fieldMap(fields)
	_, alone := fm["btn_hub_alone"]
	_, withClass := fm["btn_hub_with_students"]
	if !alone && !withClass {
		if _, ok := fm["btn_hub_cancel"]; ok {
			c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
		}
		return
	}
	if withClass {
		moved := 0
		for _, name := range c.studentsInClassWorlds(classID) {
			if p := proxy.Find(name); p != nil && c.hopPlayer(p, c.cfg.LobbyServer) == nil {
				moved++
			}
		}
		c.notify(cc, T("@1 students sent back to the HUB.", moved))
	}
	if cc.ServerName() != c.cfg.LobbyServer {
		if result := c.handleLobbyCmd(cc); result != "" {
			c.notify(cc, result)
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
	if !hasPrefixKey(fm, "del_class_") {
		c.disarmDelete(name)
	}

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
			if c.armDelete(name, "class:"+idStr) {
				_, msg := c.deleteClass(name, id)
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

	// Student list filter and scroll position (sent with every submit). A
	// changed filter is its own event: redraw and stop.
	view := c.studentList(cc.Name(), classID)
	c.mu.Lock()
	if v, ok := scrollValue(fm["scr_students"]); ok {
		view.Scroll["scr_students"] = v
	}
	filterChanged := false
	if idx, err := strconv.Atoi(fm["cls_filter"]); err == nil {
		groups, _ := c.getGroups(classID)
		_, keys := filterOptions(groups, classViewFixedFilters)
		if idx >= 1 && idx <= len(keys) && keys[idx-1] != view.ClassFilter {
			view.ClassFilter = keys[idx-1]
			view.Scroll["scr_students"] = 0
			filterChanged = true
		}
	}
	c.mu.Unlock()
	if filterChanged {
		c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
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

	if _, ok := fm["btn_people"]; ok {
		c.showStudentEditor(cc, classID)
		return
	}
	if _, ok := fm["btn_manage_students"]; ok {
		c.showStudentEditor(cc, classID)
		return
	}
	for k := range fm {
		if strings.HasPrefix(k, "join_inst_") {
			instanceID := strings.TrimPrefix(k, "join_inst_")
			inst, err := c.getInstanceByID(instanceID)
			if err == nil && inst != nil && inst.Status == "running" && inst.ClassID != nil && *inst.ClassID == classID {
				if cc.ServerName() != inst.ProxyName {
					if err := c.hopPlayer(cc, inst.ProxyName); err != nil {
						c.notify(cc, T("Could not join the class world: @1", err.Error()))
					}
				}
			} else {
				c.notify(cc, T("That class world is no longer available."))
			}
			return
		}
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
		}
		return
	}

	if _, ok := fm["btn_create_instance"]; ok {
		if !c.isAdmin(cc.Name()) {
			if msg := c.worldLimitMessage(classID); msg != "" {
				c.notify(cc, msg)
				c.showClassViewWithOrigin(cc, classID, c.getActiveClassOrigin(cc.Name()))
				return
			}
		}
		c.showTemplatePicker(cc, &classID)
		return
	}
	if _, ok := fm["btn_minigames"]; ok && canManage {
		c.openMinigames(cc, classID)
		return
	}
	if _, ok := fm["btn_return_hub"]; ok {
		// Ask only when there are students to bring back.
		if len(c.studentsInClassWorlds(classID)) > 0 {
			c.showReturnHubDialog(cc, classID)
			return
		}
		if result := c.handleLobbyCmd(cc); result != "" {
			c.notify(cc, result)
		}
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
		if strings.HasPrefix(k, "cur_rules_") || strings.HasPrefix(k, "cur_world_") {
			instID := strings.TrimPrefix(strings.TrimPrefix(k, "cur_rules_"), "cur_world_")
			inst, err := c.getInstanceByID(instID)
			if err != nil || inst == nil || inst.ClassID == nil || *inst.ClassID != classID {
				c.showClassView(cc, classID)
				return
			}
			c.setActiveInstanceWithOrigin(cc.Name(), inst.ID, c.getActiveClassOrigin(cc.Name()))
			if strings.HasPrefix(k, "cur_rules_") {
				c.showInstanceSettings(cc, inst.ID)
			} else {
				c.showWorldControls(cc, inst.ID)
			}
			return
		}
		if strings.HasPrefix(k, "bring_inst_") {
			inst, err := c.getInstanceByID(strings.TrimPrefix(k, "bring_inst_"))
			if err == nil && inst != nil && inst.Status == "running" && inst.ClassID != nil && *inst.ClassID == classID {
				c.hopClassToInstance(cc, inst, true)
			} else {
				c.notify(cc, T("That class world is not running."))
			}
			return
		}
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
				c.notify(cc, T("Instance name is required."))
				c.showTemplatePicker(cc, activeClassPtr(hasClass, classID))
				return
			}
			player := cc.Name()
			if hasClass && !c.isAdmin(player) {
				if msg := c.worldLimitMessage(classID); msg != "" {
					c.notify(cc, msg)
					c.showTemplatePicker(cc, activeClassPtr(hasClass, classID))
					return
				}
			}
			if !c.beginOp(player) {
				c.notify(cc, T("Another server operation is already running. Please wait for it to finish."))
				return
			}

			var pClassID *int
			if hasClass {
				classIDCopy := classID
				pClassID = &classIDCopy
			}

			c.showInstanceProgress(cc, T("Creating server"), T("Provisioning @1.", displayName))

			go func() {
				defer c.endOp(player)

				inst, err := c.provisionInstance(pClassID, player, tName, displayName)
				liveCC := proxy.Find(player)
				if liveCC == nil {
					return
				}
				if err != nil {
					c.showInstanceError(liveCC, inst, T("Server creation failed"), err.Error())
					return
				}
				c.showInstanceReady(liveCC, inst, T("Server ready"))
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
	if _, ok := fm["btn_inst_delete"]; !ok {
		c.disarmDelete(cc.Name())
	}

	if _, ok := fm["btn_back"]; ok {
		c.showParentForInstance(cc, inst)
		return
	}
	if c.handleInstanceTabs(cc, fm, inst) {
		return
	}

	if _, ok := fm["btn_inst_start"]; ok && inst != nil {
		player := cc.Name()
		if !c.beginOp(player) {
			c.notify(cc, T("Another server operation is already running. Please wait for it to finish."))
			return
		}

		c.showInstanceProgress(cc, T("Starting server"), T("Starting @1.", inst.ProxyName))
		go func() {
			defer c.endOp(player)

			if err := c.startInstance(inst); err != nil {
				if liveCC := proxy.Find(player); liveCC != nil {
					c.showInstanceError(liveCC, inst, T("Server start failed"), err.Error())
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
			c.showInstanceReady(liveCC, inst, T("Server ready"))
		}()
		return
	}

	if _, ok := fm["btn_inst_stop"]; ok && inst != nil {
		c.notify(cc, T("Stopping instance..."))
		go func() {
			if err := c.stopInstance(inst); err != nil {
				c.notify(cc, T("Stop failed: @1", err.Error()))
			}
		}()
		c.showInstanceViewWithOrigin(cc, instID, origin)
		return
	}

	if _, ok := fm["btn_inst_delete"]; ok && inst != nil {
		if !c.armDelete(cc.Name(), "instance:"+inst.ID) {
			c.showInstanceViewWithOrigin(cc, instID, origin)
			return
		}
		c.notify(cc, T("Deleting instance..."))
		go func() {
			if err := c.deleteInstance(inst); err != nil {
				c.notify(cc, T("Delete failed: @1", err.Error()))
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
			c.notify(cc, T("World controls are only available for classroom instances."))
			c.showInstanceViewWithOrigin(cc, instID, origin)
			return
		}
		if cc.ServerName() != inst.ProxyName {
			c.notify(cc, T("Hop to this instance server before using world controls."))
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
			c.notify(cc, T("Invited @1", invitee))
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

	if c.handleInstanceTabs(cc, fm, inst) {
		return
	}

	settingChanged := false
	needsRestart := false
	settings, err := c.getInstanceSettingsOrDefault(inst.ID)
	if err != nil {
		c.notify(cc, T("Could not load settings: @1", err.Error()))
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	if v, ok := fm["setting_damage"]; ok && boolField(v) != settings.EnableDamage {
		settings.EnableDamage = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_pvp"]; ok && boolField(v) != settings.EnablePVP {
		settings.EnablePVP = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_hunger"]; ok && boolField(v) != settings.EnableHunger {
		settings.EnableHunger = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_student_fly"]; ok && boolField(v) != settings.StudentFly {
		settings.StudentFly = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_locked"]; ok && boolField(v) != settings.WorldLocked {
		settings.WorldLocked = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_student_creative"]; ok && boolField(v) != settings.StudentCreative {
		settings.StudentCreative = boolField(v)
		settingChanged = true
	}
	if v, ok := fm["setting_mobs"]; ok && boolField(v) != settings.MobsSpawn {
		settings.MobsSpawn = boolField(v)
		settingChanged = true
		needsRestart = true
	}
	if v, ok := fm["setting_peaceful"]; ok && boolField(v) != settings.OnlyPeacefulMobs {
		settings.OnlyPeacefulMobs = boolField(v)
		settingChanged = true
		needsRestart = true
	}
	if v, ok := fm["setting_explosions"]; ok && boolField(v) != settings.ExplosionsGriefing {
		settings.ExplosionsGriefing = boolField(v)
		settingChanged = true
		needsRestart = true
	}

	if _, ok := fm["btn_back"]; ok {
		c.showInstanceViewWithOrigin(cc, inst.ID, c.getActiveInstanceOrigin(cc.Name()))
		return
	}
	if _, ok := fm["btn_capture_spawn"]; ok {
		if cc.ServerName() != inst.ProxyName {
			c.notify(cc, T("Hop to this instance before saving your position as spawn."))
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		if !cc.IsModChanJoined(modChannel) {
			go c.ensureChannelJoin(cc)
			c.notify(cc, T("Instance control channel is still connecting. Try again in a moment."))
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
		c.notify(cc, T("Asked the server to capture your current position."))
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if _, ok := fm["btn_save_settings"]; ok {
		if err := c.saveInstanceSettings(settings); err != nil {
			c.notify(cc, T("Could not save settings: @1", err.Error()))
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		if c.sendSettingsToInstance(inst, settings) {
			c.notify(cc, T("Settings saved. Hurt, PvP, hunger, flying and creative apply immediately; mobs and explosions need a restart."))
		} else {
			c.notify(cc, T("Settings saved. Start or join the instance before applying them to luanti.conf."))
		}
		c.showInstanceSettings(cc, inst.ID)
		return
	}
	if _, ok := fm["btn_reset_settings"]; ok {
		safe := defaultInstanceSettings(inst.ID)
		safe.StaticSpawnpoint, safe.SpawnYaw, safe.SpawnPitch = settings.StaticSpawnpoint, settings.SpawnYaw, settings.SpawnPitch
		needsRestart = needsRestart || safe.MobsSpawn != settings.MobsSpawn ||
			safe.OnlyPeacefulMobs != settings.OnlyPeacefulMobs || safe.ExplosionsGriefing != settings.ExplosionsGriefing
		settings = safe
		settingChanged = true
	}
	// Every change is saved and pushed at once: hurt, PvP, hunger, flying and
	// creative apply live (also to players joining later); mobs and
	// explosions are written as pending settings for the next restart, which
	// sends the settings again before restarting.
	if settingChanged {
		if err := c.saveInstanceSettings(settings); err != nil {
			c.notify(cc, T("Could not save setting: @1", err.Error()))
			c.showInstanceSettings(cc, inst.ID)
			return
		}
		c.sendSettingsToInstance(inst, settings)
		if needsRestart {
			c.setRestartPending(inst.ID, true)
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
	if c.handleInstanceTabs(cc, fm, inst) {
		return
	}
	if _, ok := fm["btn_hop_me"]; ok && inst != nil {
		if inst.Status == "running" && cc.ServerName() != inst.ProxyName {
			_ = c.hopPlayer(cc, inst.ProxyName)
		}
		return
	}
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
			c.notify(cc, T("Another server operation is already running. Please wait for it to finish."))
			return
		}
		displaced := c.playersOnInstance(inst.ProxyName)
		c.showInstanceProgress(cc, T("Applying settings"), T("Moving players to lobby, restarting, then returning them."))
		go func() {
			defer c.endOp(player)
			if _, err := c.guidedRestartInstance(inst); err != nil {
				if liveCC := proxy.Find(player); liveCC != nil {
					c.showInstanceError(liveCC, inst, T("Restart failed"), err.Error())
				}
				return
			}
			liveCC := proxy.Find(player)
			if liveCC == nil {
				return
			}
			c.setRestartPending(inst.ID, false)
			c.notify(liveCC, T("Restart complete. Returned @1 displaced players.", len(displaced)))
			updated, err := c.getInstanceByID(inst.ID)
			if err == nil && updated != nil {
				inst = updated
			}
			c.showInstanceReady(liveCC, inst, T("Settings applied"))
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
	if !hasPrefixKey(fm, "del_class_") && !hasPrefixKey(fm, "del_inst_") &&
		!hasPrefixKey(fm, "del_institute_") && !hasPrefixKey(fm, "del_account_") {
		c.disarmDelete(cc.Name())
	}

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

	if _, ok := fm["btn_admin_tab_institutes"]; ok {
		c.showAdminPanelTab(cc, "institutes")
		return
	}

	if _, ok := fm["btn_admin_add_institute"]; ok {
		_, msg := c.createInstitute(fm["new_institute_name"], fm["new_institute_max_classes"], fm["new_institute_max_worlds"])
		c.notify(cc, msg)
		c.showAdminPanelTab(cc, "institutes")
		return
	}

	if _, ok := fm["btn_admin_tab_users"]; ok {
		c.showAdminPanelTab(cc, "users")
		return
	}

	if c.handleAdminUsers(cc, fm) {
		return
	}

	if c.applyAdminFilters(cc, fm) {
		return
	}

	if _, ok := fm["btn_admin_filter_clear"]; ok {
		c.setAdminFilters(cc.Name(), "", "")
		c.showAdminPanelTab(cc, c.getAdminTab(cc.Name()))
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "open_inst_") {
			instID := strings.TrimPrefix(k, "open_inst_")
			c.showInstanceViewWithOrigin(cc, instID, viewOriginAdminInstances)
			return
		}
		if strings.HasPrefix(k, "info_inst_") {
			if inst, _ := c.getInstanceByID(strings.TrimPrefix(k, "info_inst_")); inst != nil {
				c.showAdminWorldVisits(cc, inst)
			}
			return
		}
		if strings.HasPrefix(k, "hop_inst_") {
			inst, _ := c.getInstanceByID(strings.TrimPrefix(k, "hop_inst_"))
			if inst == nil || inst.Status != "running" {
				c.notify(cc, T("This world is not running: start it from Manage."))
				c.showAdminPanelTab(cc, "instances")
				return
			}
			if cc.ServerName() != inst.ProxyName {
				if err := c.hopPlayer(cc, inst.ProxyName); err != nil {
					c.notify(cc, T("Failed to enter the world: @1", err.Error()))
				}
			}
			return
		}
		if strings.HasPrefix(k, "del_inst_") {
			instID := strings.TrimPrefix(k, "del_inst_")
			if c.armDelete(cc.Name(), "instance:"+instID) {
				if inst, _ := c.getInstanceByID(instID); inst != nil {
					c.notify(cc, T("Deleting instance..."))
					c.deleteInstancesAsync([]instanceData{*inst})
				}
			}
			c.showAdminPanelTab(cc, "instances")
			return
		}
		if strings.HasPrefix(k, "save_institute_") {
			id := strings.TrimPrefix(k, "save_institute_")
			n, _ := strconv.Atoi(id)
			_, msg := c.updateInstitute(n, fm["institute_name_"+id], fm["institute_max_classes_"+id], fm["institute_max_worlds_"+id])
			c.notify(cc, msg)
			c.showAdminPanelTab(cc, "institutes")
			return
		}
		if strings.HasPrefix(k, "del_institute_") {
			id := strings.TrimPrefix(k, "del_institute_")
			if c.armDelete(cc.Name(), "institute:"+id) {
				n, _ := strconv.Atoi(id)
				_, msg := c.deleteInstitute(n)
				c.notify(cc, msg)
			}
			c.showAdminPanelTab(cc, "institutes")
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
			if c.armDelete(cc.Name(), "class:"+idStr) {
				_, msg := c.deleteClass(cc.Name(), id)
				c.notify(cc, msg)
			}
			c.showAdminPanelTab(cc, "classes")
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
	if c.handlePeopleTabs(cc, fm, classID) {
		return
	}

	// Search and scroll position are sent with every submit.
	view := c.studentList(cc.Name(), classID)
	c.mu.Lock()
	if v, ok := scrollValue(fm["scr_people_students"]); ok {
		view.Scroll["scr_people_students"] = v
	}
	searchChanged := false // a new search starts from the top
	if v, ok := fm["stu_search"]; ok && strings.TrimSpace(v) != view.PeopleSearch {
		view.PeopleSearch, searchChanged = strings.TrimSpace(v), true
	}
	if _, ok := fm["stu_search_clear"]; ok {
		view.PeopleSearch, searchChanged = "", true
	}
	if searchChanged {
		view.Scroll["scr_people_students"] = 0
	}
	c.mu.Unlock()
	_, searchGo := fm["stu_search_go"]
	_, searchClear := fm["stu_search_clear"]
	if searchGo || searchClear || fm["key_enter_field"] == "stu_search" {
		c.showStudentEditor(cc, classID)
		return
	}

	if _, ok := fm["btn_create_student"]; ok {
		c.notify(cc, c.createStudentAccount(classID, cc.Name(), strings.TrimSpace(fm["new_student_name"]), fm["new_student_password"], fm["new_student_confirm"]))
		c.showStudentEditor(cc, classID)
		return
	}

	if _, ok := fm["btn_add_student"]; ok || fm["key_enter_field"] == "add_student_name" {
		sName := strings.TrimSpace(fm["add_student_name"])
		if ok, msg := c.addStudent(classID, sName); !ok {
			c.notify(cc, msg)
		}
		c.showStudentEditor(cc, classID)
		return
	}

	for k := range fm {
		if strings.HasPrefix(k, "pw_student_") {
			c.setPasswordTarget(cc.Name(), passwordTarget{Name: strings.TrimPrefix(k, "pw_student_"), ClassID: classID})
			c.showPasswordDialog(cc)
			return
		}
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
	if c.handlePeopleTabs(cc, fm, classID) {
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

// ── Password Dialog Handler ─────────────────────────────────────────────────

func (c *controller) handlePasswordDialog(cc *proxy.ClientConn, fields []mt.Field) {
	target, ok := c.getPasswordTarget(cc.Name())
	if !ok {
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["btn_password_cancel"]; ok {
		c.closePasswordDialog(cc)
		return
	}
	_, save := fm["btn_password_save"]
	if !save && fm["key_enter_field"] != "new_password_confirm" {
		// Closed with Escape: forget the target.
		if _, ok := fm["quit"]; ok {
			c.clearPasswordTarget(cc.Name())
		}
		return
	}
	ok, msg := c.changeAccountPassword(cc.Name(), target.Name, fm["new_password"], fm["new_password_confirm"])
	c.notify(cc, msg)
	if !ok {
		c.showPasswordDialog(cc)
		return
	}
	c.closePasswordDialog(cc)
}

// closePasswordDialog returns to the screen the dialog was opened from.
func (c *controller) closePasswordDialog(cc *proxy.ClientConn) {
	target, _ := c.getPasswordTarget(cc.Name())
	c.clearPasswordTarget(cc.Name())
	if target.ClassID != 0 {
		c.showStudentEditor(cc, target.ClassID)
		return
	}
	c.showAdminPanelTab(cc, "users")
}

func (c *controller) handleAdminWorldVisits(cc *proxy.ClientConn, fields []mt.Field) {
	if !c.isAdmin(cc.Name()) {
		return
	}
	if _, ok := fieldMap(fields)["btn_back"]; ok {
		c.showAdminPanelTab(cc, "instances")
	}
}

// applyAdminFilters reacts to the institute and teacher dropdowns of the
// Worlds and Classes tabs, which submit the form when changed.
func (c *controller) applyAdminFilters(cc *proxy.ClientConn, fm map[string]string) bool {
	tab := c.getAdminTab(cc.Name())
	if tab != "instances" && tab != "classes" {
		return false
	}
	institutes, teachers := c.adminFilterValues()
	institute, teacher := c.getAdminFilters(cc.Name())
	newInstitute, newTeacher := institute, teacher
	if idx, ok := scrollValue(fm["admin_filter_institute"]); ok && idx >= 1 && idx <= len(institutes) {
		newInstitute = institutes[idx-1]
	}
	if idx, ok := scrollValue(fm["admin_filter_teacher"]); ok && idx >= 1 && idx <= len(teachers) {
		newTeacher = teachers[idx-1]
	}
	if newInstitute == institute && newTeacher == teacher {
		return false
	}
	c.setAdminFilters(cc.Name(), newInstitute, newTeacher)
	c.showAdminPanelTab(cc, tab)
	return true
}
