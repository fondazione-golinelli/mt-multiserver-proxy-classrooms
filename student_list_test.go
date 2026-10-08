package main

import (
	"reflect"
	"testing"
)

func TestFilterStudents(t *testing.T) {
	red := classGroup{ID: 1, Name: "Red"}
	blue := classGroup{ID: 2, Name: "Blue"}
	students := []string{"Alice", "bob", "Chiara", "davide"}
	byStudent := map[string]classGroup{"Alice": red, "bob": blue, "Chiara": red}

	cases := []struct {
		filter, search string
		want           []string
	}{
		{"all", "", students},
		{"none", "", []string{"davide"}},
		{"g:1", "", []string{"Alice", "Chiara"}},
		{"g:2", "", []string{"bob"}},
		{"all", "CHI", []string{"Chiara"}},
		{"g:1", "ali", []string{"Alice"}},
		{"g:2", "ali", nil},
		{"online", "", nil}, // nobody is connected in tests
	}
	for _, tc := range cases {
		got := filterStudents(students, byStudent, tc.filter, tc.search)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("filter %q search %q: got %v, want %v", tc.filter, tc.search, got, tc.want)
		}
	}
}

func TestFilterOptionsAndScroll(t *testing.T) {
	labels, keys := filterOptions([]classGroup{{ID: 7, Name: "Red"}}, groupEditorFixedFilters)
	if labels[3] != "Group: Red" || keys[3] != "g:7" || indexOf(keys, "g:7") != 4 || indexOf(keys, "g:99") != 1 {
		t.Fatalf("unexpected options %v %v", labels, keys)
	}
	for in, want := range map[string]int{"CHG:12": 12, "VAL:3": 3, "0": 0} {
		if got, ok := scrollValue(in); !ok || got != want {
			t.Errorf("scrollValue(%q) = %d, %v", in, got, ok)
		}
	}
	if _, ok := scrollValue(""); ok {
		t.Error("empty scroll value must be rejected")
	}
}
