package main

import "testing"

func TestWorldTimePresetValue(t *testing.T) {
	tests := []struct {
		name     string
		preset   string
		want     float64
		wantStop bool
	}{
		{name: "day", preset: "day", want: 0.25},
		{name: "evening", preset: "evening", want: 0.6},
		{name: "night", preset: "night", want: 0.0},
		{name: "stop", preset: "stop", wantStop: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, stop := worldTimePresetValue(tt.preset)
			if tt.wantStop {
				if !stop {
					t.Fatalf("expected stop flag for %q", tt.preset)
				}
				return
			}
			if stop {
				t.Fatalf("did not expect stop flag for %q", tt.preset)
			}
			if got != tt.want {
				t.Fatalf("worldTimePresetValue(%q) = %v, want %v", tt.preset, got, tt.want)
			}
		})
	}
}

func TestWorldWeatherPresetValue(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		want   string
	}{
		{name: "sunny", preset: "sunny", want: "none"},
		{name: "rain", preset: "rain", want: "rain"},
		{name: "default", preset: "unknown", want: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worldWeatherPresetValue(tt.preset)
			if got != tt.want {
				t.Fatalf("worldWeatherPresetValue(%q) = %q, want %q", tt.preset, got, tt.want)
			}
		})
	}
}

func TestShouldResumeWorldTime(t *testing.T) {
	tests := []struct {
		name               string
		preset, weather    string
		stopTime, expected bool
	}{
		{name: "resume button", expected: true},
		{name: "rain is weather only", weather: "rain"},
		{name: "sunny is weather only", weather: "sunny"},
		{name: "day preset", preset: "day"},
		{name: "stop button", preset: "stop", stopTime: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldResumeWorldTime(tt.preset, tt.weather, tt.stopTime); got != tt.expected {
				t.Fatalf("shouldResumeWorldTime(%q, %q, %v) = %v, want %v",
					tt.preset, tt.weather, tt.stopTime, got, tt.expected)
			}
		})
	}
}
