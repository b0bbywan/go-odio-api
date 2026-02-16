package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/login1"
)

// TestHandleLogin1Error tests the error-to-status mapping - MOST CRITICAL TEST
func TestHandleLogin1Error(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:           "no error returns 202 Accepted",
			err:            nil,
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "CapabilityError returns 403 Forbidden",
			err:            &login1.CapabilityError{Required: "reboot capability disabled"},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "action not allowed",
		},
		{
			name:           "generic error returns 500 Internal Server Error",
			err:            http.ErrServerClosed,
			wantStatusCode: http.StatusInternalServerError,
			wantBodyMatch:  "Server closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleLogin1Error(w, tt.err)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}
			if tt.wantBodyMatch != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", body, tt.wantBodyMatch)
				}
			}
		})
	}
}

// TestWithLogin1 tests that the helper correctly wires a func() error to handleLogin1Error
func TestWithLogin1(t *testing.T) {
	tests := []struct {
		name           string
		fn             func() error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:           "success returns 202 Accepted",
			fn:             func() error { return nil },
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "CapabilityError returns 403 Forbidden",
			fn:             func() error { return &login1.CapabilityError{Required: "reboot capability disabled"} },
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "action not allowed",
		},
		{
			name:           "generic error returns 500 Internal Server Error",
			fn:             func() error { return http.ErrServerClosed },
			wantStatusCode: http.StatusInternalServerError,
			wantBodyMatch:  "Server closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withLogin1(tt.fn)
			req := httptest.NewRequest("POST", "/power/reboot", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}
			if tt.wantBodyMatch != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", body, tt.wantBodyMatch)
				}
			}
		})
	}
}

// TestPowerCapabilitiesHandler tests GET /power capability flags in the JSON response
func TestPowerCapabilitiesHandler(t *testing.T) {
	tests := []struct {
		name         string
		canReboot    bool
		canPoweroff  bool
		wantReboot   bool
		wantPowerOff bool
	}{
		{
			name:         "all capabilities disabled",
			canReboot:    false,
			canPoweroff:  false,
			wantReboot:   false,
			wantPowerOff: false,
		},
		{
			name:         "reboot only",
			canReboot:    true,
			canPoweroff:  false,
			wantReboot:   true,
			wantPowerOff: false,
		},
		{
			name:         "power_off only",
			canReboot:    false,
			canPoweroff:  true,
			wantReboot:   false,
			wantPowerOff: true,
		},
		{
			name:         "all capabilities enabled",
			canReboot:    true,
			canPoweroff:  true,
			wantReboot:   true,
			wantPowerOff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &login1.Login1Backend{
				CanReboot:   tt.canReboot,
				CanPoweroff: tt.canPoweroff,
			}
			handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
				return map[string]bool{
					"reboot":    b.CanReboot,
					"power_off": b.CanPoweroff,
				}, nil
			})

			req := httptest.NewRequest("GET", "/power", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var got map[string]bool
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if got["reboot"] != tt.wantReboot {
				t.Errorf("reboot = %v, want %v", got["reboot"], tt.wantReboot)
			}
			if got["power_off"] != tt.wantPowerOff {
				t.Errorf("power_off = %v, want %v", got["power_off"], tt.wantPowerOff)
			}
		})
	}
}

// TestRebootHandler tests POST /power/reboot - capability gate must be enforced
func TestRebootHandler(t *testing.T) {
	tests := []struct {
		name           string
		rebootFn       func() error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:           "reboot allowed returns 202 Accepted",
			rebootFn:       func() error { return nil },
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "reboot disabled returns 403 Forbidden",
			rebootFn:       func() error { return &login1.CapabilityError{Required: "reboot capability disabled"} },
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "action not allowed",
		},
		{
			name:           "D-Bus error returns 500 Internal Server Error",
			rebootFn:       func() error { return http.ErrServerClosed },
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withLogin1(tt.rebootFn)
			req := httptest.NewRequest("POST", "/power/reboot", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}
			if tt.wantBodyMatch != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", body, tt.wantBodyMatch)
				}
			}
		})
	}
}

// TestPowerOffHandler tests POST /power/power_off - capability gate must be enforced
func TestPowerOffHandler(t *testing.T) {
	tests := []struct {
		name           string
		powerOffFn     func() error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:           "power_off allowed returns 202 Accepted",
			powerOffFn:     func() error { return nil },
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "power_off disabled returns 403 Forbidden",
			powerOffFn:     func() error { return &login1.CapabilityError{Required: "poweroff capability disabled"} },
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "action not allowed",
		},
		{
			name:           "D-Bus error returns 500 Internal Server Error",
			powerOffFn:     func() error { return http.ErrServerClosed },
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withLogin1(tt.powerOffFn)
			req := httptest.NewRequest("POST", "/power/power_off", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}
			if tt.wantBodyMatch != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", body, tt.wantBodyMatch)
				}
			}
		})
	}
}
