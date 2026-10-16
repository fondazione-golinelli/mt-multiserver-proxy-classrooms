package main

import (
	"fmt"
	"strings"

	"github.com/HimbeerserverDE/mt"
	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Admin panel: shared state, cascading filters and dialogs ────────────────

// Size of every admin panel tab.
const adminW, adminH = 18.0, 11.4

// Class filter of the Worlds tab.
const (
	classFilterAll  = 0
	classFilterNone = -1 // worlds without a class
)

// adminFormState is what an administrator has typed or chosen in the admin
// panel besides the institute/teacher filters and the Users tab.
type adminFormState struct {
	ClassFilter     int    // Worlds tab: classFilterAll, classFilterNone or a class ID
	InstituteSearch string // Institutes tab
	NewUser         newUserDraft
	NewClass        newClassDraft
	Confirm         adminConfirm
}

type newUserDraft struct {
	Name           string
	Role           string
	InstituteID    int
	ClassID        int
	AdminConfirmed bool
}

type newClassDraft struct {
	Name        string
	InstituteID int
	Owner       string
}

// adminConfirm is the administrator grant waiting for confirmation.
type adminConfirm struct {
	Name    string
	NewUser bool // from the new user dialog: confirming only picks the role
}

func (c *controller) adminForm(player string) *adminFormState {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.runtime.adminForms[player]
	if f == nil {
		f = &adminFormState{NewUser: newUserDraft{Role: roleUser}}
		c.runtime.adminForms[player] = f
	}
	return f
}

// updateAdminForm changes the admin form state under the lock.
func (c *controller) updateAdminForm(player string, fn func(f *adminFormState)) {
	f := c.adminForm(player)
	c.mu.Lock()
	fn(f)
	c.mu.Unlock()
}

func (c *controller) adminFormCopy(player string) adminFormState {
	f := c.adminForm(player)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return *f
}

// adminHeaderAction draws the action button of a tab in the header.
func adminHeaderAction(b *strings.Builder, name, label, tip string) {
	b.WriteString(styledBtn(adminW-4.1, 0.2, 3.0, 0.6, name, label, colorPrimary))
	b.WriteString(tooltip(name, tip))
}

// ── Cascading filters ───────────────────────────────────────────────────────

// classesOfInstitute keeps the classes of the institute filter value: ""
// keeps every class, noInstituteFilter the classes without an institute.
func classesOfInstitute(classes []classInfo, institute string) []classInfo {
	if institute == "" {
		return classes
	}
	want := instituteFilterValue(institute)
	var out []classInfo
	for _, cl := range classes {
		if cl.InstituteName == want {
			out = append(out, cl)
		}
	}
	return out
}

// adminClassFilterValues are the Worlds tab class choices for an institute
// filter, in dropdown order: all, none, then the classes.
func (c *controller) adminClassFilterValues(institute string) (labels []string, values []int) {
	list, _ := c.loadClassInfos()
	labels = []string{T("All classes"), T("No class")}
	values = []int{classFilterAll, classFilterNone}
	for _, cl := range classesOfInstitute(list, institute) {
		labels = append(labels, cl.Name)
		values = append(values, cl.ID)
	}
	return labels, values
}

// adminTeacherValues are the Classes tab teacher choices for an institute
// filter: "" (all), then names.
func (c *controller) adminTeacherValues(institute string) []string {
	var names []string
	switch institute {
	case "":
		names, _ = c.queryStrings(`SELECT username FROM teachers
			UNION SELECT created_by FROM classes
			UNION SELECT created_by FROM instances WHERE status != 'deleted' ORDER BY 1`)
	case noInstituteFilter:
		names, _ = c.queryStrings(`SELECT username FROM teachers WHERE institute_id IS NULL
			UNION SELECT cl.created_by FROM classes cl LEFT JOIN teachers t ON t.username = cl.created_by
				WHERE t.institute_id IS NULL ORDER BY 1`)
	default:
		names, _ = c.queryStrings(`SELECT t.username FROM teachers t JOIN institutes i ON i.id = t.institute_id
			WHERE i.name = ? ORDER BY 1`, institute)
	}
	return append([]string{""}, names...)
}

// teachersOfInstitute are the possible owners of a new class: the teachers
// of the institute, or for no institute the teachers without one and the
// administrator creating it.
func (c *controller) teachersOfInstitute(instituteID int, admin string) []string {
	if instituteID != 0 {
		names, _ := c.queryStrings("SELECT username FROM teachers WHERE institute_id = ? ORDER BY username", instituteID)
		return names
	}
	names, _ := c.queryStrings("SELECT username FROM teachers WHERE institute_id IS NULL ORDER BY username")
	for _, n := range names {
		if n == admin {
			return names
		}
	}
	return append([]string{admin}, names...)
}

// filterInstitutes keeps the institutes whose name contains search.
func filterInstitutes(institutes []instituteData, search string) []instituteData {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return institutes
	}
	var out []instituteData
	for _, in := range institutes {
		if strings.Contains(strings.ToLower(in.Name), search) {
			out = append(out, in)
		}
	}
	return out
}

// ── New user dialog ─────────────────────────────────────────────────────────
//
// Password fields cannot be filled in again by the server, so the dialog is
// only redrawn when a confirmation interrupts it: dropdown changes are kept
// by the client.

func (c *controller) showNewUserDialog(cc *proxy.ClientConn) {
	d := c.adminFormCopy(cc.Name()).NewUser
	classList, _ := c.loadClassInfos()
	institutes, _ := c.listInstitutes()

	var b strings.Builder
	dialogFrame(&b, 11, 9.3, accent, iconPlus, T("New user"))

	b.WriteString(box(0.5, 1.5, 10, 2.05, colorCard))
	b.WriteString(coloredLbl(0.8, 1.85, warning, "1"))
	b.WriteString(coloredLbl(1.1, 1.85, light, T("Role")))
	roleItems := make([]string, len(editableRoles))
	roleIdx := 1
	for i, r := range editableRoles {
		roleItems[i] = roleLabel(r)
		if r == d.Role {
			roleIdx = i + 1
		}
	}
	b.WriteString(dropdownOf(0.8, 2.15, 4.3, 0.6, "nu_role", roleItems, roleIdx))
	b.WriteString(hint(5.35, 2.3, T("Administrators can manage everything:")))
	b.WriteString(hint(5.35, 2.7, T("you will be asked to confirm.")))
	b.WriteString(hint(0.8, 3.2, T("Teachers belong to an institute, students and assistance to a class.")))

	b.WriteString(box(0.5, 3.75, 10, 1.75, colorCard))
	b.WriteString(coloredLbl(0.8, 4.1, warning, "2"))
	b.WriteString(coloredLbl(1.1, 4.1, light, T("Institute (teachers)")))
	b.WriteString(instituteDropdown(0.8, 4.4, 4.5, 0.6, "nu_institute", institutes, d.InstituteID))
	b.WriteString(coloredLbl(5.6, 4.1, light, T("Class (students, assistance)")))
	classItems := []string{T("No class")}
	classIdx := 1
	for i, cl := range classList {
		label := cl.Name
		if cl.InstituteName != "" {
			label += " · " + cl.InstituteName
		}
		classItems = append(classItems, label)
		if cl.ID == d.ClassID {
			classIdx = i + 2
		}
	}
	b.WriteString(dropdownOf(5.6, 4.4, 4.6, 0.6, "nu_class", classItems, classIdx))

	b.WriteString(box(0.5, 5.7, 10, 2.3, colorCard))
	b.WriteString(coloredLbl(0.8, 6.05, warning, "3"))
	b.WriteString(coloredLbl(1.1, 6.05, light, T("Account")))
	b.WriteString(fmt.Sprintf("field[0.8,6.65;3.0,0.65;nu_name;%s;%s]", fmtEsc(T("Username")), fmtEsc(d.Name)))
	b.WriteString("field_close_on_enter[nu_name;false]")
	b.WriteString("pwdfield[3.95,6.65;3.0,0.65;nu_password;" + fmtEsc(T("Password (8+ chars)")) + "]")
	b.WriteString("pwdfield[7.1,6.65;3.1,0.65;nu_confirm;" + fmtEsc(T("Repeat password")) + "]")
	b.WriteString("field_close_on_enter[nu_password;false]field_close_on_enter[nu_confirm;false]")
	b.WriteString(hint(0.8, 7.65, T("1-20 letters, digits, _ or -")))

	b.WriteString(styledBtn(0.5, 8.3, 4.9, 0.7, "nu_create", T("Create user"), colorPrimary))
	b.WriteString(btn(5.6, 8.3, 4.9, 0.7, "nu_cancel", T("Cancel")))
	cc.ShowFormspec("classrooms:admin_new_user", b.String())
}

// readNewUserDraft keeps the dropdowns and the name of the dialog.
func (c *controller) readNewUserDraft(cc *proxy.ClientConn, fm map[string]string) newUserDraft {
	classList, _ := c.loadClassInfos()
	institutes, _ := c.listInstitutes()
	var d newUserDraft
	c.updateAdminForm(cc.Name(), func(f *adminFormState) {
		if v, ok := fm["nu_name"]; ok {
			f.NewUser.Name = strings.TrimSpace(v)
		}
		if idx, ok := scrollValue(fm["nu_role"]); ok && idx >= 1 && idx <= len(editableRoles) {
			f.NewUser.Role = editableRoles[idx-1]
		}
		if f.NewUser.Role != roleAdmin {
			f.NewUser.AdminConfirmed = false
		}
		if id, ok := instituteFromIndex(fm["nu_institute"], institutes); ok {
			f.NewUser.InstituteID = id
		}
		if id, ok := classFromIndex(fm["nu_class"], classList); ok {
			f.NewUser.ClassID = id
		}
		d = f.NewUser
	})
	return d
}

func (c *controller) handleNewUserDialog(cc *proxy.ClientConn, fields []mt.Field) {
	if !c.isAdmin(cc.Name()) {
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["nu_cancel"]; ok {
		c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewUser = newUserDraft{Role: roleUser} })
		c.showAdminPanelTab(cc, "users")
		return
	}
	if _, ok := fm["quit"]; ok {
		return
	}
	d := c.readNewUserDraft(cc, fm)
	if d.Role == roleAdmin && !d.AdminConfirmed {
		c.askAdminConfirm(cc, d.Name, true)
		return
	}
	_, create := fm["nu_create"]
	if !create && fm["key_enter_field"] == "" {
		return // a dropdown changed: the client keeps what was typed
	}

	if (d.Role == roleStudent || d.Role == roleAssistance) && d.ClassID == 0 {
		c.notify(cc, T("Choose a class for @1.", d.Name))
		return
	}
	if d.Role == roleStudent {
		if ok, msg := c.validateStudentAssignment(d.ClassID, d.Name); !ok {
			c.notify(cc, msg)
			return
		}
	}
	if msg := registerStudentAccount(proxy.DefaultAuth(), d.Name, fm["nu_password"], fm["nu_confirm"]); msg != "" {
		c.notify(cc, msg)
		return
	}
	if err := c.recordAccountCreator(d.Name, cc.Name()); err != nil {
		c.notify(cc, T("Database error."))
	}
	msg := T("Account @1 created.", d.Name)
	if d.Role != roleUser {
		_, classes := c.loadClassInfos()
		row := c.loadUserRows(classes)[d.Name]
		if row == nil {
			row = &userRow{Name: d.Name, Registered: true, Role: roleUser}
		}
		_, roleMsg := c.setUserRole(cc.Name(), row, d.Role, d.InstituteID, d.ClassID)
		msg += " " + roleMsg
	}
	c.notify(cc, msg)

	// Show the new account in the Users tab.
	c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewUser = newUserDraft{Role: roleUser} })
	v := c.adminUsers(cc.Name())
	c.mu.Lock()
	v.Search, v.Role, v.InstituteID, v.ClassID, v.Scroll = d.Name, "", -1, -1, 0
	c.mu.Unlock()
	c.showAdminPanelTab(cc, "users")
}

// ── Administrator confirmation ──────────────────────────────────────────────

func (c *controller) askAdminConfirm(cc *proxy.ClientConn, name string, newUser bool) {
	c.updateAdminForm(cc.Name(), func(f *adminFormState) {
		f.Confirm = adminConfirm{Name: name, NewUser: newUser}
	})
	who := name
	if who == "" {
		who = T("the new user")
	}
	var b strings.Builder
	dialogFrame(&b, 10, 6.2, warning, iconWarning, T("Make @1 an administrator?", who))
	b.WriteString(box(0.5, 1.6, 9, 2.75, colorCard))
	b.WriteString(coloredLbl(0.85, 2.05, light, T("Administrators can:")))
	b.WriteString(hint(0.85, 2.55, "•  "+T("create, change and delete institutes, classes and worlds")))
	b.WriteString(hint(0.85, 3.0, "•  "+T("create and delete accounts and change any password")))
	b.WriteString(hint(0.85, 3.45, "•  "+T("make other users administrators or remove them")))
	b.WriteString(hint(0.85, 3.95, T("Give this role only to people who run the service.")))
	b.WriteString(styledBtn(0.5, 4.8, 5.2, 0.8, "adm_confirm", T("Yes, make administrator"), colorActive))
	b.WriteString(btn(5.9, 4.8, 3.6, 0.8, "adm_cancel", T("Cancel")))
	cc.ShowFormspec("classrooms:admin_confirm", b.String())
}

func (c *controller) handleAdminConfirm(cc *proxy.ClientConn, fields []mt.Field) {
	if !c.isAdmin(cc.Name()) {
		return
	}
	fm := fieldMap(fields)
	pending := c.adminFormCopy(cc.Name()).Confirm
	_, yes := fm["adm_confirm"]
	_, no := fm["adm_cancel"]
	if !yes && !no {
		return
	}
	if pending.NewUser {
		c.updateAdminForm(cc.Name(), func(f *adminFormState) {
			if yes {
				f.NewUser.AdminConfirmed = true
			} else {
				f.NewUser.Role = roleUser
			}
		})
		c.showNewUserDialog(cc)
		return
	}
	if yes && pending.Name != "" {
		_, msg := c.grantPanelAdmin(cc.Name(), pending.Name)
		c.notify(cc, msg)
	}
	v := c.adminUsers(cc.Name())
	c.mu.Lock()
	delete(v.Draft, pending.Name)
	c.mu.Unlock()
	c.showAdminPanelTab(cc, "users")
}

// ── New class dialog ────────────────────────────────────────────────────────

func (c *controller) showNewClassDialog(cc *proxy.ClientConn) {
	d := c.adminFormCopy(cc.Name()).NewClass
	institutes, _ := c.listInstitutes()
	owners := c.teachersOfInstitute(d.InstituteID, cc.Name())

	var b strings.Builder
	dialogFrame(&b, 10, 7.3, accent, iconPlus, T("New class"))
	b.WriteString(box(0.5, 1.5, 9, 4.6, colorCard))
	b.WriteString(fmt.Sprintf("field[0.8,2.15;8.4,0.65;nc_name;%s;%s]", fmtEsc(T("Class name, e.g. 3B Science")), fmtEsc(d.Name)))
	b.WriteString("field_close_on_enter[nc_name;false]")
	b.WriteString(coloredLbl(0.8, 3.2, light, T("Institute")))
	b.WriteString(instituteDropdown(0.8, 3.5, 4.1, 0.6, "nc_institute", institutes, d.InstituteID))
	b.WriteString(coloredLbl(5.1, 3.2, light, T("Owner teacher")))
	if len(owners) == 0 {
		b.WriteString(disabledBtn(5.1, 3.5, 4.1, 0.6, T("No teachers"), ""))
		b.WriteString(hint(0.8, 4.55, T("This institute has no teachers yet:")))
		b.WriteString(hint(0.8, 4.95, T("create one in the Users tab first.")))
	} else {
		items := make([]string, len(owners))
		idx := 1
		for i, o := range owners {
			items[i] = o
			if o == cc.Name() {
				items[i] = T("Me (@1)", o)
			}
			if o == d.Owner {
				idx = i + 1
			}
		}
		b.WriteString(dropdownOf(5.1, 3.5, 4.1, 0.6, "nc_owner", items, idx))
		b.WriteString(hint(0.8, 4.55, T("The class counts towards the institute of its owner.")))
		b.WriteString(hint(0.8, 4.95, T("Administrators are not bound by the institute limits.")))
	}
	if len(owners) > 0 {
		b.WriteString(styledBtn(0.5, 6.35, 4.4, 0.7, "nc_create", T("Create class"), colorPrimary))
	}
	b.WriteString(btn(5.1, 6.35, 4.4, 0.7, "nc_cancel", T("Cancel")))
	cc.ShowFormspec("classrooms:admin_new_class", b.String())
}

func (c *controller) handleNewClassDialog(cc *proxy.ClientConn, fields []mt.Field) {
	if !c.isAdmin(cc.Name()) {
		return
	}
	fm := fieldMap(fields)
	if _, ok := fm["nc_cancel"]; ok {
		c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewClass = newClassDraft{} })
		c.showAdminPanelTab(cc, "classes")
		return
	}
	if _, ok := fm["quit"]; ok {
		return
	}
	institutes, _ := c.listInstitutes()
	before := c.adminFormCopy(cc.Name()).NewClass
	var d newClassDraft
	c.updateAdminForm(cc.Name(), func(f *adminFormState) {
		if v, ok := fm["nc_name"]; ok {
			f.NewClass.Name = strings.TrimSpace(v)
		}
		if id, ok := instituteFromIndex(fm["nc_institute"], institutes); ok {
			f.NewClass.InstituteID = id
		}
		d = f.NewClass
	})
	owners := c.teachersOfInstitute(before.InstituteID, cc.Name())
	if idx, ok := scrollValue(fm["nc_owner"]); ok && idx >= 1 && idx <= len(owners) {
		d.Owner = owners[idx-1]
	}
	if d.InstituteID != before.InstituteID {
		// Other institute, other teachers.
		d.Owner = ""
		c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewClass = d })
		c.showNewClassDialog(cc)
		return
	}
	c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewClass = d })
	if _, ok := fm["nc_create"]; !ok && fm["key_enter_field"] != "nc_name" {
		return
	}
	if d.Owner == "" && len(owners) > 0 {
		d.Owner = owners[0]
	}
	if d.Owner == "" {
		c.notify(cc, T("Choose the owner teacher of the class."))
		return
	}
	ok, msg := c.insertClass(d.Owner, d.Name)
	c.notify(cc, msg)
	if !ok {
		return
	}
	// Show the institute of the new class in the Classes tab.
	institute := noInstituteFilter
	for _, in := range institutes {
		if in.ID == d.InstituteID {
			institute = in.Name
		}
	}
	c.setAdminFilters(cc.Name(), institute, "")
	c.updateAdminForm(cc.Name(), func(f *adminFormState) { f.NewClass = newClassDraft{} })
	c.showAdminPanelTab(cc, "classes")
}
