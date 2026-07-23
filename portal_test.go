package main

import "testing"

func TestShouldUsePortalSpectator(t *testing.T) {
	inst := &instanceData{CreatedBy: "owner"}
	tests := []struct {
		name, player      string
		admin, classStaff bool
		want              bool
	}{
		{name: "generic visitor", player: "visitor", want: true},
		{name: "student still spectates", player: "student", want: true},
		{name: "unrelated teacher spectates", player: "other-teacher", want: true},
		{name: "owner keeps staff role", player: "owner", want: false},
		{name: "class staff keeps staff role", player: "assistant", classStaff: true, want: false},
		{name: "admin keeps admin role", player: "admin", admin: true, want: false},
		{name: "missing instance is safe", player: "visitor", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := inst
			if tt.name == "missing instance is safe" {
				target = nil
			}
			if got := shouldUsePortalSpectator(target, tt.player, tt.admin, tt.classStaff); got != tt.want {
				t.Fatalf("shouldUsePortalSpectator() = %v, want %v", got, tt.want)
			}
		})
	}
}
