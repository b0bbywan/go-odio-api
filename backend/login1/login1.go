package login1

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New creates a new Login1 backend
func New(ctx context.Context, cfg *config.Login1Config) (*Login1Backend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	backend := &Login1Backend{
		conn:    conn,
		ctx:     ctx,
		timeout: 10 * time.Second,
	}

	if cfg.Capabilities != nil {
		if !cfg.Capabilities.CanReboot && !cfg.Capabilities.CanPoweroff {
			logger.Warn("[login1] no capability enabled, disabling backend")
			return nil, nil
		}
		if err := backend.validateCapabilities(*cfg.Capabilities); err != nil {
			logger.Error("[login1] failed to validate capabilities: %v", err)
			backend.Close()
			return nil, err
		}
	}

	logger.Info("[login1] backend initialized")
	return backend, nil
}

// Close cleanly closes connections and stops the listener
func (l *Login1Backend) Close() {
	if l.conn != nil {
		if err := l.conn.Close(); err != nil {
			logger.Error("Failed to close D-Bus connection: %v", err)
		}
		l.conn = nil
	}
}

func (l *Login1Backend) Reboot() error {
	if !l.CanReboot {
		return &CapabilityError{Required: "reboot capability disabled"}
	}
	logger.Info("[login1] Reboot requested")
	return l.callMethod(LOGIN1_PREFIX, LOGIN1_METHOD_REBOOT, true)
}

func (l *Login1Backend) PowerOff() error {
	if !l.CanPoweroff {
		return &CapabilityError{Required: "poweroff capability disabled"}
	}
	logger.Info("[login1] PowerOff requested")
	return l.callMethod(LOGIN1_PREFIX, LOGIN1_METHOD_POWEROFF, true)
}

func (l *Login1Backend) validateCapabilities(capabilities config.Login1Capabilities) error {
	// test valid capabilities or return nil
	if capabilities.CanReboot {
		if err := l.validateCapability(LOGIN1_CAPABILITY_REBOOT); err != nil {
			return err
		}
		l.CanReboot = true
	}

	if capabilities.CanPoweroff {
		if err := l.validateCapability(LOGIN1_CAPABILITY_POWEROFF); err != nil {
			return err
		}
		l.CanPoweroff = true
	}

	return nil
}

func (l *Login1Backend) checkCapability(method string) (bool, error) {
	call, err := l.callDBusMethod(method)
	if err != nil {
		return false, err
	}

	result, err := extractString(call)
	if err != nil {
		return false, err
	}

	return result == "yes", nil
}

func (l *Login1Backend) validateCapability(method string) error {
	available, err := l.checkCapability(method)
	if err != nil {
		logger.Error("[login1] capability check failed: method %s, error: %v", method, err)
		return fmt.Errorf("%s check failed: %w", method, err)
	}
	if !available {
		logger.Error("[login1] capability not available method %s", method)
		return &CapabilityError{Required: method + " not available"}
	}
	return nil
}
