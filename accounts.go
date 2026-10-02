package main

import (
	"regexp"
	"strings"
	"unicode/utf8"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
	"github.com/HimbeerserverDE/srp"
)

// Structural interface keeps the plugin build-compatible with older proxies.
// Those proxies must be upgraded before account creation can be used.
type accountCreator interface {
	CreateAccount(name string, salt, verifier []byte) error
}

var accountNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,20}$`)

func validateAccountInput(name, password, confirmation string) string {
	if !accountNamePattern.MatchString(name) || name == "singleplayer" {
		return "Username must contain 1-20 letters, digits, underscores or hyphens; singleplayer is reserved."
	}
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 128 {
		return "Password must contain 8-128 characters."
	}
	if password != confirmation {
		return "Passwords do not match."
	}
	return ""
}

func registerStudentAccount(auth proxy.AuthBackend, name, password, confirmation string) string {
	if msg := validateAccountInput(name, password, confirmation); msg != "" {
		return msg
	}
	creator, ok := auth.(accountCreator)
	if !ok {
		return "Account creation is unavailable. Update the proxy or its authentication backend."
	}
	if auth.Exists(name) {
		return "This account already exists. Use Add to assign it without changing its password."
	}
	// Luanti lowercases the SRP identity, while preserving the login name.
	salt, verifier, err := srp.NewClient([]byte(strings.ToLower(name)), []byte(password))
	if err != nil {
		return "Unable to generate account credentials."
	}
	if err := creator.CreateAccount(name, salt, verifier); err != nil {
		return "Account creation failed or the username is already taken. No existing password was changed."
	}
	return ""
}

func (c *controller) createStudentAccount(classID int, actor, name, password, confirmation string) string {
	if !c.canManageClass(classID, actor) {
		return "Only class teachers and administrators can create accounts."
	}
	if msg := validateAccountInput(name, password, confirmation); msg != "" {
		return msg
	}
	if ok, msg := c.validateStudentAssignment(classID, name); !ok {
		return msg
	}
	if msg := registerStudentAccount(proxy.DefaultAuth(), name, password, confirmation); msg != "" {
		return msg
	}
	if ok, _ := c.addStudent(classID, name); !ok {
		// Authentication and class membership use separate stores. Keep the account
		// and report partial success, so the teacher can retry the existing Add flow.
		return "Account created, but assignment failed. Use Add with this username to retry; the password is already set."
	}
	return "Account created and student added to the class."
}
