package main

import (
	"errors"
	"log"
	"os"
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
		return T("Username must contain 1-20 letters, digits, underscores or hyphens; singleplayer is reserved.")
	}
	return validatePassword(password, confirmation)
}

func validatePassword(password, confirmation string) string {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 128 {
		return T("Password must contain 8-128 characters.")
	}
	if password != confirmation {
		return T("Passwords do not match.")
	}
	return ""
}

func registerStudentAccount(auth proxy.AuthBackend, name, password, confirmation string) string {
	if msg := validateAccountInput(name, password, confirmation); msg != "" {
		return msg
	}
	creator, ok := auth.(accountCreator)
	if !ok {
		return T("Account creation is unavailable. Update the proxy or its authentication backend.")
	}
	if auth.Exists(name) {
		return T("This account already exists. Use Add to assign it without changing its password.")
	}
	// Luanti lowercases the SRP identity, while preserving the login name.
	salt, verifier, err := srp.NewClient([]byte(strings.ToLower(name)), []byte(password))
	if err != nil {
		return T("Unable to generate account credentials.")
	}
	if err := creator.CreateAccount(name, salt, verifier); err != nil {
		return T("Account creation failed or the username is already taken. No existing password was changed.")
	}
	return ""
}

func (c *controller) createStudentAccount(classID int, actor, name, password, confirmation string) string {
	if !c.canManageClass(classID, actor) {
		return T("Only class teachers and administrators can create accounts.")
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
	if err := c.recordAccountCreator(name, actor); err != nil {
		log.Printf("[%s] failed to record creator of account %s: %v", pluginName, name, err)
	}
	if ok, _ := c.addStudent(classID, name); !ok {
		// Authentication and class membership use separate stores. Keep the account
		// and report partial success, so the teacher can retry the existing Add flow.
		return T("Account created, but assignment failed. Use Add with this username to retry; the password is already set.")
	}
	return T("Account created and student added to the class.")
}

// ── Password changes ────────────────────────────────────────────────────────

// recordAccountCreator remembers who created an account from the class panel,
// which is what lets a teacher change that account's password later.
func (c *controller) recordAccountCreator(name, creator string) error {
	_, err := c.db.Exec(`INSERT INTO account_creators (username, created_by) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE created_by = VALUES(created_by), created_at = CURRENT_TIMESTAMP`, name, creator)
	return err
}

func (c *controller) getAccountCreator(name string) string {
	var creator string
	if err := c.db.QueryRow("SELECT created_by FROM account_creators WHERE username = ?", name).Scan(&creator); err != nil {
		return ""
	}
	return creator
}

// getAccountCreators maps account names to their creator. An empty creator
// returns every recorded account.
func (c *controller) getAccountCreators(creator string) map[string]string {
	query, args := "SELECT username, created_by FROM account_creators", []any{}
	if creator != "" {
		query, args = query+" WHERE created_by = ?", append(args, creator)
	}
	result := make(map[string]string)
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var name, by string
		if rows.Scan(&name, &by) == nil {
			result[name] = by
		}
	}
	return result
}

// hasStaffRole reports roles whose password only administrators may change,
// even if a teacher created the account. It also works for offline players.
func (c *controller) hasStaffRole(name string) bool {
	if _, ok := proxy.Conf().UserGroups[name]; ok {
		return true
	}
	if ok, _ := c.getTeacher(name); ok {
		return true
	}
	return c.isAssistant(name) || c.isAdmin(name)
}

// canChangePassword: administrators may change any password, teachers only
// those of the student accounts they created themselves.
func (c *controller) canChangePassword(actor, target string) bool {
	if c.isAdmin(actor) {
		return true
	}
	return actor != target && c.isTeacher(actor) &&
		c.getAccountCreator(target) == actor && !c.hasStaffRole(target)
}

func setAccountPassword(auth proxy.AuthBackend, name, password, confirmation string) string {
	if msg := validatePassword(password, confirmation); msg != "" {
		return msg
	}
	if auth == nil || !auth.Exists(name) {
		return T("This account does not exist.")
	}
	// Same SRP identity as registerStudentAccount.
	salt, verifier, err := srp.NewClient([]byte(strings.ToLower(name)), []byte(password))
	if err != nil {
		return T("Unable to generate account credentials.")
	}
	if err := auth.SetPasswd(name, salt, verifier); err != nil {
		return T("Changing the password failed.")
	}
	return ""
}

func (c *controller) changeAccountPassword(actor, target, password, confirmation string) (bool, string) {
	if !c.canChangePassword(actor, target) {
		return false, T("You can only change the password of student accounts you created.")
	}
	if msg := setAccountPassword(proxy.DefaultAuth(), target, password, confirmation); msg != "" {
		return false, msg
	}
	log.Printf("[%s] %s changed the password of %s", pluginName, actor, target)
	return true, T("Password of @1 changed. It applies from the next login.", target)
}

// allAccountNames lists the files backend by its directory: its Export reads
// every password, which also moves every account's last login time.
func allAccountNames(auth proxy.AuthBackend) ([]string, error) {
	if auth == nil {
		return nil, errors.New("no authentication backend")
	}
	if _, ok := auth.(proxy.AuthFiles); ok {
		entries, err := os.ReadDir(proxy.Path("auth"))
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names, nil
	}
	users, err := auth.Export()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Name)
	}
	return names, nil
}

// ── Account deletion ────────────────────────────────────────────────────────

// accountDeleter is implemented by authentication backends that can delete
// accounts. The proxy's built-in backends don't, so the files backend, which
// Skyss uses, is handled by deleteAuthAccount directly.
type accountDeleter interface {
	DeleteAccount(name string) error
}

var errAccountDeletionUnsupported = errors.New("account deletion unsupported by the authentication backend")

// safeAccountName also guards the files backend path against traversal.
var safeAccountName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,20}$`)

func deleteAuthAccount(auth proxy.AuthBackend, name string) error {
	if !safeAccountName.MatchString(name) {
		return errors.New("invalid account name")
	}
	if d, ok := auth.(accountDeleter); ok {
		return d.DeleteAccount(name)
	}
	if _, ok := auth.(proxy.AuthFiles); ok {
		return os.RemoveAll(proxy.Path("auth/", name))
	}
	return errAccountDeletionUnsupported
}

// deleteUserAccount deletes a login and every classroom record about it.
// Owners of classes must delete those classes first.
func (c *controller) deleteUserAccount(actor, name string) (bool, string) {
	if !c.isAdmin(actor) {
		return false, T("Only administrators can delete accounts.")
	}
	if name == actor {
		return false, T("You cannot delete your own account.")
	}
	if _, ok := proxy.Conf().UserGroups[name]; ok {
		return false, T("@1 has a proxy permission group: remove it from the proxy configuration first.", name)
	}
	var owned int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM classes WHERE created_by = ?", name).Scan(&owned); err != nil {
		return false, T("Database error.")
	}
	if owned > 0 {
		return false, T("@1 owns @2 classes: delete them first.", name, owned)
	}
	// Names with a classroom role but no login only lose their records.
	if auth := proxy.DefaultAuth(); auth != nil && auth.Exists(name) {
		if cc := proxy.Find(name); cc != nil {
			cc.Kick(T("Your account has been deleted."))
		}
		if err := deleteAuthAccount(auth, name); err != nil {
			log.Printf("[%s] failed to delete account %s: %v", pluginName, name, err)
			if errors.Is(err, errAccountDeletionUnsupported) {
				return false, T("Account deletion is unavailable with this authentication backend.")
			}
			return false, T("Deleting the account failed.")
		}
	}
	for _, stmt := range []string{
		"DELETE FROM class_students WHERE username = ?",
		"DELETE FROM class_assistants WHERE username = ?",
		"DELETE FROM class_teachers WHERE username = ?",
		"DELETE FROM teachers WHERE username = ?",
		"DELETE FROM instance_invites WHERE username = ?",
		"DELETE FROM account_creators WHERE username = ? OR created_by = ?",
	} {
		args := []any{name}
		if strings.Count(stmt, "?") == 2 {
			args = append(args, name)
		}
		if _, err := c.db.Exec(stmt, args...); err != nil {
			log.Printf("[%s] account %s deleted, but cleanup failed (%s): %v", pluginName, name, stmt, err)
		}
	}
	c.mu.Lock()
	delete(c.runtime.frozenPlayers, name)
	delete(c.runtime.watchingPlayers, name)
	c.mu.Unlock()

	log.Printf("[%s] %s deleted the account %s", pluginName, actor, name)
	return true, T("Account @1 deleted.", name)
}

// shortList joins names, keeping the line short enough for one row.
func shortList(names []string) string {
	const max = 30
	s := strings.Join(names, ", ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max-1]) + "…"
	}
	return s
}
