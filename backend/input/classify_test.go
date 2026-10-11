package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassify(t *testing.T) {
	forced := map[string]bool{"flirc.tv flirc": true}

	tests := []struct {
		name       string
		deviceName string
		parentLink string
		target     string
		bustype    string
		wantKind   Kind
		wantOK     bool
	}{
		{
			name:       "rc-core receiver",
			deviceName: "gpio_ir_recv", parentLink: "subsystem", target: "../../../../class/rc", bustype: "0019",
			wantKind: KindIR, wantOK: true,
		},
		{
			name:       "rc-core device without a bus type",
			deviceName: "gpio_ir_recv", parentLink: "subsystem", target: "../../../../class/rc",
		},
		{
			name:       "rc-core device with an unreadable bus type",
			deviceName: "gpio_ir_recv", parentLink: "subsystem", target: "../../../../class/rc", bustype: "zz",
		},
		{
			name:       "hdmi-cec adapter",
			deviceName: "vc4-hdmi", parentLink: "subsystem", target: "../../../../class/rc", bustype: "001e",
			wantKind: KindCEC, wantOK: true,
		},
		{
			name:       "cec bus outside rc-core",
			deviceName: "some keyboard", parentLink: "subsystem", target: "../../../../bus/hid", bustype: "001e",
		},
		{
			name:       "gpio-keys",
			deviceName: "gpio-keys", parentLink: "driver", target: "../../../bus/platform/drivers/gpio-keys",
			wantKind: KindGPIO, wantOK: true,
		},
		{
			name:       "polled gpio-keys",
			deviceName: "buttons", parentLink: "driver", target: "../../../bus/platform/drivers/gpio-keys-polled",
			wantKind: KindGPIO, wantOK: true,
		},
		{
			name:       "listed by name, case-insensitive",
			deviceName: "Flirc.tv FLIRC", parentLink: "subsystem", target: "../../../../bus/hid",
			wantKind: KindForced, wantOK: true,
		},
		{
			name:       "listed name wins over the IR rule",
			deviceName: "flirc.tv flirc", parentLink: "subsystem", target: "../../../../class/rc",
			wantKind: KindForced, wantOK: true,
		},
		{
			name:       "usb keyboard",
			deviceName: "Logitech USB Keyboard", parentLink: "subsystem", target: "../../../../bus/hid",
		},
		{
			name:       "platform hotkeys",
			deviceName: "ThinkPad Extra Buttons", parentLink: "driver", target: "../../../bus/platform/drivers/thinkpad_acpi",
		},
		{name: "no parent device", deviceName: "Power Button"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sysDir := filepath.Join(t.TempDir(), "event0")
			parent := filepath.Join(sysDir, "device", "device")
			if err := os.MkdirAll(parent, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sysDir, "device", "name"), []byte(tt.deviceName+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if tt.bustype != "" {
				idDir := filepath.Join(sysDir, "device", "id")
				if err := os.MkdirAll(idDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(idDir, "bustype"), []byte(tt.bustype+"\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.parentLink != "" {
				if err := os.Symlink(tt.target, filepath.Join(parent, tt.parentLink)); err != nil {
					t.Fatal(err)
				}
			}

			name, kind, ok := classify(sysDir, forced)
			if name != tt.deviceName || kind != tt.wantKind || ok != tt.wantOK {
				t.Errorf("classify() = (%q, %q, %t), want (%q, %q, %t)",
					name, kind, ok, tt.deviceName, tt.wantKind, tt.wantOK)
			}
		})
	}
}

func TestClassifyMissingNode(t *testing.T) {
	if name, kind, ok := classify(filepath.Join(t.TempDir(), "event9"), nil); ok {
		t.Errorf("classify() = (%q, %q, true), want not handled", name, kind)
	}
}
