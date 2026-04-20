package main

import (
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// isAllowedToHop checks if a player is permitted to join a specific server.
func (c *controller) isAllowedToHop(playerName, serverName string) bool {
	// If it's a static server (not an instance), always allow.
	// Instances are identified by their status not being 'deleted'.
	inst, err := c.getInstanceByProxyName(serverName)
	if err != nil {
		// Log error and deny for safety if DB is having issues
		return false
	}

	// If no instance record found, it's likely a static server (lobby, etc.)
	if inst == nil {
		return true
	}

	// Instance creator is always allowed
	if inst.CreatedBy == playerName {
		return true
	}

	// Admins are always allowed
	if c.isAdmin(playerName) {
		return true
	}

	// If instance is bound to a class, class students are allowed
	if inst.ClassID != nil {
		if c.isStudentInClass(*inst.ClassID, playerName) {
			return true
		}
	}

	// Check for explicit invites
	if c.isInstanceInvited(inst.ID, playerName) {
		return true
	}

	return false
}

// onChatMsg intercepts /join, >join and other slash commands to enforce access control.
func (c *controller) onChatMsg(cc *proxy.ClientConn, msg string) string {
	// 1. Access Control for Join
	if strings.HasPrefix(msg, "/join ") || strings.HasPrefix(msg, ">join ") {
		parts := strings.Split(msg, " ")
		if len(parts) >= 2 {
			serverName := parts[1]
			if cc.ServerName() == serverName {
				cc.SendChatMsg("[Classrooms] You are already on that server.")
				return ""
			}
			if !c.isAllowedToHop(cc.Name(), serverName) {
				cc.SendChatMsg("[Classrooms] You are not permitted to join this instance.")
				return ""
			}
			// Success! Re-apply states after hop.
			// The proxy will perform the hop after this hook returns.
			go func(name string) {
				time.Sleep(3 * time.Second) // Wait for hop to complete
				c.reapplyStates(name)
			}(cc.Name())
		}
		return msg
	}

	// 2. Slash command interceptors
	if msg == "/classes" {
		if res := c.handleClassesCmd(cc); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if msg == "/admin" {
		if res := c.handleAdminCmd(cc); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if msg == "/lobby" {
		if res := c.handleLobbyCmd(cc); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if strings.HasPrefix(msg, "/freeze ") {
		if res := c.handleFreezeCmd(cc, strings.Split(msg, " ")[1:]...); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if strings.HasPrefix(msg, "/unfreeze ") {
		if res := c.handleUnfreezeCmd(cc, strings.Split(msg, " ")[1:]...); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if strings.HasPrefix(msg, "/teacher_add ") {
		if res := c.handleTeacherAddCmd(cc, strings.Split(msg, " ")[1:]...); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if strings.HasPrefix(msg, "/teacher_remove ") {
		if res := c.handleTeacherRemoveCmd(cc, strings.Split(msg, " ")[1:]...); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}
	if msg == "/teacher_list" {
		if res := c.handleTeacherListCmd(cc); res != "" {
			cc.SendChatMsg(res)
		}
		return ""
	}

	return msg
}
