package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Admin Users tab ─────────────────────────────────────────────────────────
//
// One table of every account and every name with a classroom role, with its
// role, institute and class. Rows are edited in place: changing the role
// dropdown redraws the row with the fields that role needs (draft), Save
// applies it. Choosing Administrator asks for confirmation and applies at
// once. "+ New user" in the header opens the new account dialog.

const (
	roleAdmin      = "admin"
	roleTeacher    = "teacher"
	roleAssistance = "assistance"
	roleStudent    = "student"
	roleUser       = "user"

	maxUserRows = 60
)

// editableRoles are the choices of a row's role dropdown, in order.
var editableRoles = []string{roleUser, roleStudent, roleAssistance, roleTeacher, roleAdmin}

// roleFilters are the choices of the role filter, after "All roles".
var roleFilters = []string{roleAdmin, roleTeacher, roleAssistance, roleStudent, roleUser}

func roleLabel(role string) string {
	switch role {
	case roleAdmin:
		return T("Administrator")
	case roleTeacher:
		return T("Teacher")
	case roleAssistance:
		return T("Assistance")
	case roleStudent:
		return T("Student")
	}
	return T("User")
}

// adminUsersView is what an administrator has set up in the Users tab.
type adminUsersView struct {
	Search      string
	Role        string // "" = all
	InstituteID int    // -1 = all, 0 = no institute
	ClassID     int    // -1 = all, 0 = no class
	Scroll      int
	Draft       map[string]string // user -> role chosen but not saved yet
}

func (c *controller) adminUsers(player string) *adminUsersView {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.runtime.adminUsers[player]
	if v == nil {
		v = &adminUsersView{InstituteID: -1, ClassID: -1, Draft: map[string]string{}}
		c.runtime.adminUsers[player] = v
	}
	return v
}

// ── Data ────────────────────────────────────────────────────────────────────

type classInfo struct {
	ID            int
	Name          string
	InstituteID   int
	InstituteName string
}

type userRow struct {
	Name           string
	Registered     bool
	ConfigAdmin    bool   // administrator from the proxy configuration
	Role           string // primary role, see primaryRole
	Creator        string
	TeacherInstID  int
	Institute      string // name used to sort, see sortInstitute
	TeacherClasses []int  // owned or linked
	StudentClass   int
	AssistClasses  []int // sorted by class name
}

// primaryRole: a name can hold several roles; the table shows the highest.
func (r *userRow) primaryRole(isTeacher, isAdmin bool) string {
	switch {
	case isAdmin:
		return roleAdmin
	case isTeacher:
		return roleTeacher
	case len(r.AssistClasses) > 0:
		return roleAssistance
	case r.StudentClass != 0:
		return roleStudent
	}
	return roleUser
}

// classIDs are all classes the user belongs to, in any role.
func (r *userRow) classIDs() []int {
	ids := append([]int{}, r.TeacherClasses...)
	ids = append(ids, r.AssistClasses...)
	if r.StudentClass != 0 {
		ids = append(ids, r.StudentClass)
	}
	return ids
}

// instituteIDs are the institutes the user belongs to: their own as a
// teacher, or through their classes.
func (r *userRow) instituteIDs(classes map[int]classInfo) []int {
	var ids []int
	if r.TeacherInstID != 0 {
		ids = append(ids, r.TeacherInstID)
	}
	for _, id := range append(append([]int{}, r.AssistClasses...), r.StudentClass) {
		if cl, ok := classes[id]; ok && cl.InstituteID != 0 {
			ids = append(ids, cl.InstituteID)
		}
	}
	return ids
}

func (c *controller) loadClassInfos() ([]classInfo, map[int]classInfo) {
	var list []classInfo
	byID := make(map[int]classInfo)
	rows, err := c.db.Query(`SELECT cl.id, cl.name, COALESCE(t.institute_id, 0), COALESCE(i.name, '') FROM classes cl
		LEFT JOIN teachers t ON t.username = cl.created_by
		LEFT JOIN institutes i ON i.id = t.institute_id ORDER BY cl.name`)
	if err != nil {
		return nil, byID
	}
	defer rows.Close()
	for rows.Next() {
		var ci classInfo
		if rows.Scan(&ci.ID, &ci.Name, &ci.InstituteID, &ci.InstituteName) == nil {
			list = append(list, ci)
			byID[ci.ID] = ci
		}
	}
	return list, byID
}

// loadUserRows returns every registered account plus every name with a
// classroom role, keyed by name.
func (c *controller) loadUserRows(classes map[int]classInfo) map[string]*userRow {
	users := make(map[string]*userRow)
	get := func(name string) *userRow {
		if users[name] == nil {
			users[name] = &userRow{Name: name}
		}
		return users[name]
	}
	if names, err := allAccountNames(proxy.DefaultAuth()); err == nil {
		for _, n := range names {
			get(n).Registered = true
		}
	}
	teachers := make(map[string]bool)
	scan := func(query string, fn func(name string, id int)) {
		rows, err := c.db.Query(query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var id int
			if rows.Scan(&name, &id) == nil {
				fn(name, id)
			}
		}
	}
	teacherInst := make(map[string]string)
	if rows, err := c.db.Query(`SELECT t.username, COALESCE(t.institute_id, 0), COALESCE(i.name, '') FROM teachers t
		LEFT JOIN institutes i ON i.id = t.institute_id`); err == nil {
		for rows.Next() {
			var name, inst string
			var id int
			if rows.Scan(&name, &id, &inst) == nil {
				teachers[name] = true
				get(name).TeacherInstID = id
				teacherInst[name] = inst
			}
		}
		rows.Close()
	}
	scan(`SELECT created_by, id FROM classes UNION SELECT username, class_id FROM class_teachers`, func(name string, id int) {
		r := get(name)
		r.TeacherClasses = append(r.TeacherClasses, id)
	})
	scan("SELECT username, class_id FROM class_students", func(name string, id int) {
		get(name).StudentClass = id
	})
	scan("SELECT username, class_id FROM class_assistants", func(name string, id int) {
		r := get(name)
		r.AssistClasses = append(r.AssistClasses, id)
	})
	for name, by := range c.getAccountCreators("") {
		if r, ok := users[name]; ok {
			r.Creator = by
		}
	}
	for _, r := range users {
		sort.Slice(r.AssistClasses, func(i, j int) bool {
			return classes[r.AssistClasses[i]].Name < classes[r.AssistClasses[j]].Name
		})
		r.ConfigAdmin = isProxyAdmin(r.Name)
		r.Role = r.primaryRole(teachers[r.Name], r.ConfigAdmin || c.isPanelAdmin(r.Name))
		r.Institute = teacherInst[r.Name]
		if r.Institute == "" && r.StudentClass != 0 {
			r.Institute = classes[r.StudentClass].InstituteName
		}
		if r.Institute == "" && len(r.AssistClasses) > 0 {
			r.Institute = classes[r.AssistClasses[0]].InstituteName
		}
	}
	return users
}

// isProxyAdmin is isAdmin for offline players too: it reads the permission
// groups of the proxy configuration.
func isProxyAdmin(name string) bool {
	if cc := proxy.Find(name); cc != nil {
		return cc.HasPerms("server")
	}
	conf := proxy.Conf()
	group, ok := conf.UserGroups[name]
	if !ok {
		group = "default"
	}
	for _, perm := range conf.Groups[group] {
		if perm == "server" || (strings.HasSuffix(perm, "*") && strings.HasPrefix("server", perm[:len(perm)-1])) {
			return true
		}
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// roleOrder sorts the Users tab: administrators first, plain users last.
var roleOrder = map[string]int{roleAdmin: 0, roleTeacher: 1, roleAssistance: 2, roleStudent: 3, roleUser: 4}

// lessUserRow orders by role, then institute (none last), then name.
func lessUserRow(a, b *userRow) bool {
	if roleOrder[a.Role] != roleOrder[b.Role] {
		return roleOrder[a.Role] < roleOrder[b.Role]
	}
	ai, bi := strings.ToLower(a.Institute), strings.ToLower(b.Institute)
	if ai != bi {
		if ai == "" || bi == "" {
			return bi == ""
		}
		return ai < bi
	}
	return strings.ToLower(a.Name) < strings.ToLower(b.Name)
}

// filterUserRows applies the filters of the view, sorted by lessUserRow.
func filterUserRows(users map[string]*userRow, classes map[int]classInfo, v *adminUsersView) []*userRow {
	search := strings.ToLower(v.Search)
	var result []*userRow
	for _, r := range users {
		if search != "" && !strings.Contains(strings.ToLower(r.Name), search) {
			continue
		}
		if v.Role != "" && r.Role != v.Role {
			continue
		}
		if v.InstituteID >= 0 {
			ids := r.instituteIDs(classes)
			if (v.InstituteID == 0 && len(ids) > 0) || (v.InstituteID > 0 && !containsInt(ids, v.InstituteID)) {
				continue
			}
		}
		if v.ClassID >= 0 {
			ids := r.classIDs()
			if (v.ClassID == 0 && len(ids) > 0) || (v.ClassID > 0 && !containsInt(ids, v.ClassID)) {
				continue
			}
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool { return lessUserRow(result[i], result[j]) })
	return result
}

// ── Form ────────────────────────────────────────────────────────────────────

func dropdownOf(x, y, w, h float64, name string, items []string, selected int) string {
	escaped := make([]string, len(items))
	for i, it := range items {
		escaped[i] = fmtEsc(it)
	}
	return fmt.Sprintf("dropdown[%g,%g;%g,%g;%s;%s;%d;true]", x, y, w, h, name, strings.Join(escaped, ","), selected)
}

// filterIDIndex maps a filter value (-1 all, 0 none, else ID) to a dropdown
// index over "All", "None", then ids.
func filterIDIndex(value int, ids []int) int {
	switch value {
	case -1:
		return 1
	case 0:
		return 2
	}
	for i, id := range ids {
		if id == value {
			return i + 3
		}
	}
	return 1
}

func filterIDFromIndex(field string, ids []int) (int, bool) {
	idx, ok := scrollValue(field)
	if !ok || idx < 1 || idx > len(ids)+2 {
		return 0, false
	}
	switch idx {
	case 1:
		return -1, true
	case 2:
		return 0, true
	}
	return ids[idx-3], true
}

// classDropdown is "No class" plus every class.
func classDropdown(x, y, w, h float64, name string, classes []classInfo, selectedID int) string {
	items := []string{T("No class")}
	selected := 1
	for i, cl := range classes {
		items = append(items, cl.Name)
		if cl.ID == selectedID {
			selected = i + 2
		}
	}
	return dropdownOf(x, y, w, h, name, items, selected)
}

func classFromIndex(field string, classes []classInfo) (int, bool) {
	idx, ok := scrollValue(field)
	if !ok || idx < 1 || idx > len(classes)+1 {
		return 0, false
	}
	if idx == 1 {
		return 0, true
	}
	return classes[idx-2].ID, true
}

func (c *controller) writeAdminUsersTab(cc *proxy.ClientConn, b *strings.Builder) {
	v := c.adminUsers(cc.Name())
	classList, classes := c.loadClassInfos()
	institutes, _ := c.listInstitutes()
	users := c.loadUserRows(classes)
	shown := filterUserRows(users, classes, v)

	instIDs := make([]int, len(institutes))
	instItems := []string{T("All institutes"), T("No institute")}
	for i, in := range institutes {
		instIDs[i] = in.ID
		instItems = append(instItems, in.Name)
	}
	classIDs := make([]int, len(classList))
	classItems := []string{T("All classes"), T("No class")}
	for i, cl := range classList {
		classIDs[i] = cl.ID
		classItems = append(classItems, cl.Name)
	}
	roleItems := []string{T("All roles")}
	roleIdx := 1
	for i, r := range roleFilters {
		roleItems = append(roleItems, roleLabel(r))
		if r == v.Role {
			roleIdx = i + 2
		}
	}

	// Filters: the dropdowns apply at once, the search with Enter.
	b.WriteString(box(0.2, 2.05, adminW-0.4, 0.8, colorCard))
	b.WriteString(fmt.Sprintf("image[0.4,2.22;0.45,0.45;%s]", iconSearch))
	b.WriteString(fmt.Sprintf("field[1.0,2.2;3.0,0.5;users_search;;%s]", fmtEsc(v.Search)))
	b.WriteString("field_close_on_enter[users_search;false]")
	b.WriteString(tooltip("users_search", T("Search by name, then press Enter")))
	b.WriteString(dropdownOf(4.15, 2.2, 2.5, 0.5, "users_filter_role", roleItems, roleIdx))
	b.WriteString(dropdownOf(6.8, 2.2, 3.1, 0.5, "users_filter_institute", instItems, filterIDIndex(v.InstituteID, instIDs)))
	b.WriteString(dropdownOf(10.05, 2.2, 3.1, 0.5, "users_filter_class", classItems, filterIDIndex(v.ClassID, classIDs)))
	b.WriteString(iconBtn(13.35, 2.15, 0.6, "users_search_go", iconSearch, T("Search")))
	b.WriteString(btn(14.15, 2.2, 2.4, 0.5, "users_filter_clear", T("Clear filters")))

	b.WriteString(hint(0.55, 3.15, T("Name")))
	b.WriteString(hint(4.05, 3.15, T("Role")))
	b.WriteString(hint(6.7, 3.15, T("Institute")))
	b.WriteString(hint(9.95, 3.15, T("Class")))
	b.WriteString(hint(13.4, 3.15, T("@1 of @2 users", len(shown), len(users))))

	if len(shown) == 0 {
		b.WriteString(hint(0.5, 3.8, T("No users match the filters.")))
		return
	}
	if len(shown) > maxUserRows {
		shown = shown[:maxUserRows]
		b.WriteString(hint(0.5, adminH-0.2, T("Only the first @1 are shown: refine the search.", maxUserRows)))
	}

	const listTop, listH, rowH, rowStep = 3.4, adminH - 3.85, 0.72, 0.8
	b.WriteString(scrollbarAt("scr_admin_users", adminW-0.5, listTop, listH, len(shown), rowStep, 0.05, v.Scroll))
	b.WriteString(fmt.Sprintf("scroll_container[0.2,%g;%g,%g;scr_admin_users;vertical;0.1]", listTop, adminW-0.75, listH))
	c.mu.RLock()
	drafts := make(map[string]string, len(v.Draft))
	for k, d := range v.Draft {
		drafts[k] = d
	}
	c.mu.RUnlock()
	y := 0.05
	for _, r := range shown {
		// Proxy-configuration administrators and your own administrator
		// role cannot be changed here.
		locked := r.ConfigAdmin || (r.Role == roleAdmin && r.Name == cc.Name())
		role, isDraft := r.Role, false
		if d, ok := drafts[r.Name]; ok && !locked {
			role, isDraft = d, d != r.Role
		}
		rowColor := colorCard
		if isDraft {
			rowColor = colorCurrent
		}
		b.WriteString(box(0, y, adminW-0.8, rowH, rowColor))
		dot := muted
		if proxy.Find(r.Name) != nil {
			dot = success
		}
		b.WriteString(statusDot(0.15, y+0.25, dot))
		nameColor := light
		if !r.Registered {
			nameColor = muted
		}
		b.WriteString(fmt.Sprintf("label[0.5,%g;%s]", y+0.36, fmtEsc(mcColorize(nameColor, r.Name))))
		var tips []string
		if !r.Registered {
			tips = append(tips, T("Not registered yet: the role applies when the account is created."))
		}
		if r.Creator != "" {
			tips = append(tips, T("Created by @1", r.Creator))
		}
		if len(tips) > 0 {
			b.WriteString(fmt.Sprintf("tooltip[0,%g;3.6,%g;%s]", y, rowH, fmtEsc(strings.Join(tips, "\n"))))
		}

		fy := y + 0.11
		if locked {
			b.WriteString(fmt.Sprintf("label[3.85,%g;%s]", y+0.36, fmtEsc(mcColorize(warning, roleLabel(roleAdmin)))))
			tip := T("You cannot remove your own administrator rights.")
			if r.ConfigAdmin {
				tip = T("Set in the proxy configuration (PROXY_ADMIN_USERS): it cannot be changed here.")
			}
			b.WriteString(fmt.Sprintf("tooltip[3.75,%g;2.7,%g;%s]", y, rowH, fmtEsc(tip)))
		} else {
			idx := 1
			for i, er := range editableRoles {
				if er == role {
					idx = i + 1
				}
			}
			items := make([]string, len(editableRoles))
			for i, er := range editableRoles {
				items[i] = roleLabel(er)
			}
			b.WriteString(dropdownOf(3.85, fy, 2.5, 0.5, "role_"+r.Name, items, idx))
		}

		muteLbl := func(x float64, text string) {
			b.WriteString(fmt.Sprintf("label[%g,%g;%s]", x, y+0.36, fmtEsc(mcColorize(muted, text))))
		}
		classNames := func(ids []int) string {
			var names []string
			for _, id := range ids {
				names = append(names, classes[id].Name)
			}
			sort.Strings(names)
			return shortList(names)
		}
		switch role {
		case roleTeacher:
			b.WriteString(instituteDropdown(6.5, fy, 3.1, 0.5, "inst_"+r.Name, institutes, r.TeacherInstID))
			if len(r.TeacherClasses) > 0 {
				muteLbl(9.75, classNames(r.TeacherClasses))
			} else {
				muteLbl(9.75, T("No classes"))
			}
		case roleStudent, roleAssistance:
			selected := r.StudentClass
			if role == roleAssistance {
				selected = 0
				if len(r.AssistClasses) > 0 {
					selected = r.AssistClasses[0]
				}
			}
			if inst := classes[selected].InstituteName; inst != "" {
				muteLbl(6.6, inst)
			} else {
				muteLbl(6.6, "—")
			}
			b.WriteString(classDropdown(9.75, fy, 3.1, 0.5, "class_"+r.Name, classList, selected))
			if role == roleAssistance && len(r.AssistClasses) > 1 {
				b.WriteString(tooltip("class_"+r.Name, T("Assistance in @1. Saving keeps only the chosen class.", classNames(r.AssistClasses))))
			}
		case roleAdmin:
			for _, in := range institutes {
				if in.ID == r.TeacherInstID {
					muteLbl(6.6, in.Name)
				}
			}
			if ids := r.classIDs(); len(ids) > 0 {
				muteLbl(9.75, classNames(ids))
			}
		default:
			muteLbl(6.6, "—")
			muteLbl(9.75, "—")
		}

		if !locked {
			saveColor := colorButton
			if isDraft {
				saveColor = colorPrimary
			}
			b.WriteString(styledBtn(13.05, fy, 1.3, 0.5, "save_user_"+r.Name, T("Save"), saveColor))
		}
		if r.Registered {
			b.WriteString(iconBtn(14.5, y+0.09, 0.54, "pw_account_"+r.Name, iconLock, T("Change password: @1", r.Name)))
		}
		if r.Role != roleAdmin && r.Name != cc.Name() {
			delName := "del_account_" + r.Name
			if c.isDeleteArmed(cc.Name(), "account:"+r.Name) {
				b.WriteString(styledBtn(15.15, fy, 0.95, 0.5, delName, T("Sure?"), colorDanger))
			} else {
				b.WriteString(iconBtn(15.3, y+0.09, 0.54, delName, iconClose, T("Delete account: @1", r.Name)))
			}
		}
		y += rowStep
	}
	b.WriteString("scroll_container_end[]")
}

// ── Handler ─────────────────────────────────────────────────────────────────

// handleAdminUsers handles the fields of the Users tab. It returns false
// when the submit was not for this tab.
func (c *controller) handleAdminUsers(cc *proxy.ClientConn, fm map[string]string) bool {
	if c.getAdminTab(cc.Name()) != "users" {
		return false
	}
	if _, ok := fm["quit"]; ok {
		return true
	}
	v := c.adminUsers(cc.Name())
	classList, classes := c.loadClassInfos()
	institutes, _ := c.listInstitutes()

	redraw := false
	c.mu.Lock()
	if n, ok := scrollValue(fm["scr_admin_users"]); ok {
		v.Scroll = n
	}
	if s, ok := fm["users_search"]; ok && strings.TrimSpace(s) != v.Search {
		_, go1 := fm["users_search_go"]
		if go1 || fm["key_enter_field"] == "users_search" {
			v.Search, v.Scroll, redraw = strings.TrimSpace(s), 0, true
		}
	}
	if idx, ok := scrollValue(fm["users_filter_role"]); ok {
		role := ""
		if idx >= 2 && idx-2 < len(roleFilters) {
			role = roleFilters[idx-2]
		}
		if role != v.Role {
			v.Role, v.Scroll, redraw = role, 0, true
		}
	}
	instIDs := make([]int, len(institutes))
	for i, in := range institutes {
		instIDs[i] = in.ID
	}
	if id, ok := filterIDFromIndex(fm["users_filter_institute"], instIDs); ok && id != v.InstituteID {
		v.InstituteID, v.Scroll, redraw = id, 0, true
	}
	classIDs := make([]int, len(classList))
	for i, cl := range classList {
		classIDs[i] = cl.ID
	}
	if id, ok := filterIDFromIndex(fm["users_filter_class"], classIDs); ok && id != v.ClassID {
		v.ClassID, v.Scroll, redraw = id, 0, true
	}
	if _, ok := fm["users_filter_clear"]; ok {
		v.Search, v.Role, v.InstituteID, v.ClassID, v.Scroll = "", "", -1, -1, 0
		redraw = true
	}
	c.mu.Unlock()

	// A changed role dropdown redraws its row with the fields of that role.
	users := c.loadUserRows(classes)
	for k, val := range fm {
		name, ok := strings.CutPrefix(k, "role_")
		if !ok || users[name] == nil {
			continue
		}
		idx, ok := scrollValue(val)
		if !ok || idx < 1 || idx > len(editableRoles) {
			continue
		}
		role := editableRoles[idx-1]
		if role == roleAdmin && users[name].Role != roleAdmin {
			c.askAdminConfirm(cc, name, false)
			return true
		}
		c.mu.Lock()
		current, hasDraft := v.Draft[name]
		if !hasDraft {
			current = users[name].Role
		}
		if role != current {
			if role == users[name].Role {
				delete(v.Draft, name)
			} else {
				v.Draft[name] = role
			}
			redraw = true
		}
		c.mu.Unlock()
	}

	for k := range fm {
		if name, ok := strings.CutPrefix(k, "save_user_"); ok {
			r := users[name]
			if r == nil {
				break
			}
			role := r.Role
			if idx, ok := scrollValue(fm["role_"+name]); ok && idx >= 1 && idx <= len(editableRoles) {
				role = editableRoles[idx-1]
			}
			instituteID, _ := instituteFromIndex(fm["inst_"+name], institutes)
			classID, _ := classFromIndex(fm["class_"+name], classList)
			if role == roleAdmin && r.Role != roleAdmin {
				c.askAdminConfirm(cc, name, false)
				return true
			}
			_, msg := c.setUserRole(cc.Name(), r, role, instituteID, classID)
			c.notify(cc, msg)
			c.mu.Lock()
			delete(v.Draft, name)
			c.mu.Unlock()
			redraw = true
		}
		if name, ok := strings.CutPrefix(k, "pw_account_"); ok {
			c.setPasswordTarget(cc.Name(), passwordTarget{Name: name})
			c.showPasswordDialog(cc)
			return true
		}
		if name, ok := strings.CutPrefix(k, "del_account_"); ok {
			if c.armDelete(cc.Name(), "account:"+name) {
				_, msg := c.deleteUserAccount(cc.Name(), name)
				c.notify(cc, msg)
			}
			redraw = true
		}
	}

	if redraw {
		c.showAdminPanelTab(cc, "users")
	}
	return true
}

// setUserRole gives a user exactly one classroom role. Teachers keep their
// class links; the other roles replace the user's class memberships. A panel
// administrator given another role loses the administrator rights; keeping
// Administrator keeps everything as it is.
func (c *controller) setUserRole(actor string, r *userRow, role string, instituteID, classID int) (bool, string) {
	name := r.Name
	if r.ConfigAdmin {
		return false, T("Administrators are managed in the proxy configuration.")
	}
	wasAdmin := c.isPanelAdmin(name)
	if role == roleAdmin {
		if wasAdmin {
			return true, T("@1 is now: @2.", name, roleLabel(roleAdmin))
		}
		return c.grantPanelAdmin(actor, name)
	}
	if wasAdmin && name == actor {
		return false, T("You cannot remove your own administrator rights.")
	}
	if (role == roleStudent || role == roleAssistance) && classID == 0 {
		return false, T("Choose a class for @1.", name)
	}
	if role != roleTeacher {
		var owned int
		if err := c.db.QueryRow("SELECT COUNT(*) FROM classes WHERE created_by = ?", name).Scan(&owned); err != nil {
			return false, T("Database error.")
		}
		if owned > 0 {
			return false, T("@1 owns @2 classes: delete them first.", name, owned)
		}
	}

	tx, err := c.db.Begin()
	if err != nil {
		return false, T("Database error.")
	}
	defer tx.Rollback()
	exec := func(query string, args ...any) bool {
		if _, err := tx.Exec(query, args...); err != nil {
			log.Printf("[%s] setUserRole %s: %v", pluginName, name, err)
			return false
		}
		return true
	}
	ok := true
	if wasAdmin {
		ok = ok && exec("DELETE FROM panel_admins WHERE username = ?", name)
	}
	if role != roleTeacher {
		ok = ok && exec("DELETE FROM teachers WHERE username = ?", name)
	}
	if role != roleStudent || r.StudentClass != classID {
		ok = ok && exec("DELETE FROM class_students WHERE username = ?", name)
	}
	// An Assistance row saved without changing its class keeps its other classes.
	keepAssist := role == roleAssistance && len(r.AssistClasses) > 0 && r.AssistClasses[0] == classID
	if !keepAssist {
		ok = ok && exec("DELETE FROM class_assistants WHERE username = ?", name)
	}
	switch role {
	case roleTeacher:
		ok = ok && exec(`INSERT INTO teachers (username, institute_id) VALUES (?, NULLIF(?, 0))
			ON DUPLICATE KEY UPDATE institute_id = VALUES(institute_id)`, name, instituteID)
	case roleStudent:
		if r.StudentClass != classID {
			ok = ok && exec("INSERT INTO class_students (class_id, username) VALUES (?, ?)", classID, name)
		}
	case roleAssistance:
		if !keepAssist {
			ok = ok && exec("INSERT INTO class_assistants (class_id, username) VALUES (?, ?)", classID, name)
		}
	}
	if !ok || tx.Commit() != nil {
		return false, T("Saving @1 failed.", name)
	}
	if wasAdmin {
		c.forgetPanelAdmin(actor, name)
	}
	if proxy.Find(name) != nil {
		c.scheduleReapplyStates(name, 0)
	}
	log.Printf("[%s] %s is now %s (class %d, institute %d)", pluginName, name, role, classID, instituteID)
	return true, T("@1 is now: @2.", name, roleLabel(role))
}
