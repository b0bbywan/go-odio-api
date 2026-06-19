package upgrade

import (
	"errors"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

func TestParseProgressContract(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"begin with total", `{"event":"begin","total":3}`, true},
		{"begin without total", `{"event":"begin"}`, false},
		{"progress complete", `{"event":"progress","percent":42,"current":1,"step":"mpd"}`, true},
		{"progress missing step", `{"event":"progress","percent":42,"current":1}`, false},
		{"progress missing current", `{"event":"progress","percent":42,"step":"mpd"}`, false},
		{"end with success", `{"event":"end","success":true}`, true},
		{"end without success", `{"event":"end"}`, false},
		{"unknown event", `{"event":"halfway"}`, false},
		{"missing event field", `{"percent":10}`, false},
		{"malformed json", `{"event":`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := parseProgress([]byte(tc.line)); ok != tc.want {
				t.Errorf("parseProgress(%s) ok = %v, want %v", tc.line, ok, tc.want)
			}
		})
	}
}

func TestStartUpgradeRejectsConcurrentRun(t *testing.T) {
	u := &UpgradeBackend{upgradeUnit: "upgrade.service", systemd: &systemd.SystemdBackend{}}
	u.run.start(sourceUnit) // a run is already in flight

	if err := u.StartUpgrade(); !errors.Is(err, ErrUpgradeInProgress) {
		t.Fatalf("StartUpgrade while running = %v, want ErrUpgradeInProgress", err)
	}
}
