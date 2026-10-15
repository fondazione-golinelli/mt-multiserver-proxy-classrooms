package main

import "testing"

func TestFilterUserRows(t *testing.T) {
	classes := map[int]classInfo{
		1: {ID: 1, Name: "3B", InstituteID: 10},
		2: {ID: 2, Name: "4A", InstituteID: 20},
	}
	users := map[string]*userRow{
		"anna":  {Name: "anna", Role: roleStudent, StudentClass: 1},
		"bruno": {Name: "bruno", Role: roleTeacher, TeacherInstID: 20, TeacherClasses: []int{2}},
		"carla": {Name: "carla", Role: roleAssistance, AssistClasses: []int{2}},
		"dario": {Name: "dario", Role: roleUser},
	}
	names := func(v *adminUsersView) string {
		var s string
		for _, r := range filterUserRows(users, classes, v) {
			s += r.Name + " "
		}
		return s
	}
	all := &adminUsersView{InstituteID: -1, ClassID: -1}
	for _, tc := range []struct {
		view *adminUsersView
		want string
	}{
		{all, "bruno carla anna dario "},
		{&adminUsersView{Role: roleStudent, InstituteID: -1, ClassID: -1}, "anna "},
		{&adminUsersView{InstituteID: 20, ClassID: -1}, "bruno carla "},
		{&adminUsersView{InstituteID: 0, ClassID: -1}, "dario "},
		{&adminUsersView{InstituteID: -1, ClassID: 2}, "bruno carla "},
		{&adminUsersView{InstituteID: -1, ClassID: 0}, "dario "},
		{&adminUsersView{Search: "AR", InstituteID: -1, ClassID: -1}, "carla dario "},
		{&adminUsersView{Role: roleStudent, InstituteID: -1, ClassID: -1}, "anna "},
	} {
		if got := names(tc.view); got != tc.want {
			t.Errorf("%+v: got %q, want %q", *tc.view, got, tc.want)
		}
	}
}

func TestPrimaryRole(t *testing.T) {
	r := &userRow{StudentClass: 1, AssistClasses: []int{2}}
	if r.primaryRole(false, false) != roleAssistance || r.primaryRole(true, false) != roleTeacher || r.primaryRole(true, true) != roleAdmin {
		t.Fatal("wrong role priority")
	}
	if (&userRow{}).primaryRole(false, false) != roleUser {
		t.Fatal("plain account is not a user")
	}
}

func TestUserRowOrder(t *testing.T) {
	rows := []*userRow{
		{Name: "zed", Role: roleStudent},
		{Name: "bea", Role: roleStudent, Institute: "Liceo B"},
		{Name: "al", Role: roleStudent, Institute: "Liceo B"},
		{Name: "carl", Role: roleStudent, Institute: "liceo a"},
		{Name: "root", Role: roleAdmin},
		{Name: "tina", Role: roleTeacher, Institute: "Z"},
	}
	users := map[string]*userRow{}
	for _, r := range rows {
		users[r.Name] = r
	}
	var got string
	for _, r := range filterUserRows(users, nil, &adminUsersView{InstituteID: -1, ClassID: -1}) {
		got += r.Name + " "
	}
	if want := "root tina carl al bea zed "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
