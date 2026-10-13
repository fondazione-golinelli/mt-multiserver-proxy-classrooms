package main

import (
	"strings"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func (c *controller) registerCommands() {
	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "classes",
		Perm:    "",
		Help:    "Open teacher dashboard",
		Usage:   "classes",
		Handler: c.handleClassesCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "admin",
		Perm:    "server",
		Help:    "Open admin dashboard",
		Usage:   "admin",
		Handler: c.handleAdminCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "lobby",
		Perm:    "",
		Help:    "Go back to the lobby",
		Usage:   "lobby",
		Handler: c.handleLobbyCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "teacher_add",
		Perm:    "server",
		Help:    "Register a new teacher",
		Usage:   "teacher_add <username>",
		Handler: c.handleTeacherAddCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "teacher_remove",
		Perm:    "server",
		Help:    "Unregister a teacher",
		Usage:   "teacher_remove <username>",
		Handler: c.handleTeacherRemoveCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "teacher_list",
		Perm:    "server",
		Help:    "List all registered teachers",
		Usage:   "teacher_list",
		Handler: c.handleTeacherListCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "freeze",
		Perm:    "",
		Help:    "Freeze a student or class",
		Usage:   "freeze <student_name|class_name>",
		Handler: c.handleFreezeCmd,
	})

	proxy.RegisterChatCmd(proxy.ChatCmd{
		Name:    "unfreeze",
		Perm:    "",
		Help:    "Unfreeze a student or class",
		Usage:   "unfreeze <student_name|class_name>",
		Handler: c.handleUnfreezeCmd,
	})
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (c *controller) handleClassesCmd(cc *proxy.ClientConn, args ...string) string {
	if !c.hasClassPanelAccess(cc.Name()) {
		return T("You do not have access to the class panel.")
	}
	c.showPanelHome(cc)
	return ""
}

func (c *controller) handleAdminCmd(cc *proxy.ClientConn, args ...string) string {
	// Perm "server" already checked by proxy
	c.showAdminPanel(cc)
	return ""
}

func (c *controller) handleLobbyCmd(cc *proxy.ClientConn, args ...string) string {
	if c.cfg.LobbyServer == "" {
		return T("Lobby not configured.")
	}
	if cc.ServerName() == c.cfg.LobbyServer {
		return T("You are already in the lobby.")
	}
	if err := c.hopPlayer(cc, c.cfg.LobbyServer); err != nil {
		return T("Failed to hop to lobby: @1", err.Error())
	}
	return ""
}

func (c *controller) handleTeacherAddCmd(cc *proxy.ClientConn, args ...string) string {
	if len(args) < 1 {
		return T("Usage: >teacher_add <username>")
	}
	name := args[0]
	if err := c.addTeacher(name); err != nil {
		return T("Error adding teacher: @1", err.Error())
	}
	if proxy.Find(name) != nil {
		c.scheduleReapplyStates(name, 0)
	}
	return T("Teacher @1 added.", name)
}

func (c *controller) handleTeacherRemoveCmd(cc *proxy.ClientConn, args ...string) string {
	if len(args) < 1 {
		return T("Usage: >teacher_remove <username>")
	}
	name := args[0]
	if err := c.removeTeacher(name); err != nil {
		return T("Error removing teacher: @1", err.Error())
	}
	if proxy.Find(name) != nil {
		c.scheduleReapplyStates(name, 0)
	}
	return T("Teacher @1 removed.", name)
}

func (c *controller) handleTeacherListCmd(cc *proxy.ClientConn, args ...string) string {
	teachers, err := c.listTeachers()
	if err != nil {
		return T("Error listing teachers: @1", err.Error())
	}
	if len(teachers) == 0 {
		return T("No teachers registered.")
	}
	return T("Teachers: @1", strings.Join(teachers, ", "))
}

func (c *controller) handleFreezeCmd(cc *proxy.ClientConn, args ...string) string {
	if !c.isTeacher(cc.Name()) {
		return T("You are not permitted to use this command.")
	}
	if len(args) < 1 {
		return T("Usage: >freeze <student_name|class_name>")
	}

	target := args[0]
	// Try class name first
	cls, err := c.getClassByName(target)
	if err == nil && cls != nil {
		c.freezeClass(cls.ID)
		return T("Class @1 frozen.", target)
	}

	// Try student name
	c.freezePlayer(target)
	return T("Player @1 frozen.", target)
}

func (c *controller) handleUnfreezeCmd(cc *proxy.ClientConn, args ...string) string {
	if !c.isTeacher(cc.Name()) {
		return T("You are not permitted to use this command.")
	}
	if len(args) < 1 {
		return T("Usage: >unfreeze <student_name|class_name>")
	}

	target := args[0]
	// Try class name first
	cls, err := c.getClassByName(target)
	if err == nil && cls != nil {
		c.unfreezeClass(cls.ID)
		return T("Class @1 unfrozen.", target)
	}

	// Try student name
	c.unfreezePlayer(target)
	return T("Player @1 unfrozen.", target)
}
