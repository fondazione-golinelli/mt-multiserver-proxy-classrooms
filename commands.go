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
	if !c.isTeacher(cc.Name()) {
		return "You are not registered as a teacher."
	}
	c.showMainDashboard(cc)
	return ""
}

func (c *controller) handleAdminCmd(cc *proxy.ClientConn, args ...string) string {
	// Perm "server" already checked by proxy
	c.showAdminPanel(cc)
	return ""
}

func (c *controller) handleLobbyCmd(cc *proxy.ClientConn, args ...string) string {
	if c.cfg.LobbyServer == "" {
		return "Lobby not configured."
	}
	if err := cc.Hop(c.cfg.LobbyServer); err != nil {
		return "Failed to hop to lobby: " + err.Error()
	}
	return ""
}

func (c *controller) handleTeacherAddCmd(cc *proxy.ClientConn, args ...string) string {
	if len(args) < 1 {
		return "Usage: >teacher_add <username>"
	}
	name := args[0]
	if err := c.addTeacher(name); err != nil {
		return "Error adding teacher: " + err.Error()
	}
	return "Teacher " + name + " added."
}

func (c *controller) handleTeacherRemoveCmd(cc *proxy.ClientConn, args ...string) string {
	if len(args) < 1 {
		return "Usage: >teacher_remove <username>"
	}
	name := args[0]
	if err := c.removeTeacher(name); err != nil {
		return "Error removing teacher: " + err.Error()
	}
	return "Teacher " + name + " removed."
}

func (c *controller) handleTeacherListCmd(cc *proxy.ClientConn, args ...string) string {
	teachers, err := c.listTeachers()
	if err != nil {
		return "Error listing teachers: " + err.Error()
	}
	if len(teachers) == 0 {
		return "No teachers registered."
	}
	return "Teachers: " + strings.Join(teachers, ", ")
}

func (c *controller) handleFreezeCmd(cc *proxy.ClientConn, args ...string) string {
	if !c.isTeacher(cc.Name()) {
		return "You are not permitted to use this command."
	}
	if len(args) < 1 {
		return "Usage: >freeze <student_name|class_name>"
	}

	target := args[0]
	// Try class name first
	cls, err := c.getClassByName(target)
	if err == nil && cls != nil {
		c.freezeClass(cls.ID)
		return "Class " + target + " frozen."
	}

	// Try student name
	c.freezePlayer(target)
	return "Player " + target + " frozen."
}

func (c *controller) handleUnfreezeCmd(cc *proxy.ClientConn, args ...string) string {
	if !c.isTeacher(cc.Name()) {
		return "You are not permitted to use this command."
	}
	if len(args) < 1 {
		return "Usage: >unfreeze <student_name|class_name>"
	}

	target := args[0]
	// Try class name first
	cls, err := c.getClassByName(target)
	if err == nil && cls != nil {
		c.unfreezeClass(cls.ID)
		return "Class " + target + " unfrozen."
	}

	// Try student name
	c.unfreezePlayer(target)
	return "Player " + target + " unfrozen."
}
