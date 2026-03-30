package mpris

import (
	"strings"

	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/logger"
)

func (m *MPRISBackend) handleSignal(sig *dbus.Signal) {
	logger.Debug("[mpris] received signal: %s from %s", sig.Name, sig.Sender)
	switch sig.Name {
	case DBUS_PROP_CHANGED_SIGNAL:
		m.handlePropertiesChanged(sig)
	case DBUS_NAME_OWNER_CHANGED:
		m.handleNameOwnerChanged(sig)
	default:
		logger.Debug("[mpris] unhandled signal: %s", sig.Name)
	}
}

func (m *MPRISBackend) handlePropertiesChanged(sig *dbus.Signal) {
	changed, iface, err := idbus.FilterSignal(sig)
	if err != nil || iface != MPRIS_PLAYER_IFACE {
		return
	}

	busName := m.findPlayerByUniqueName(sig.Sender)
	if busName == "" {
		return
	}

	if status := idbus.MapString(changed, "PlaybackStatus"); status != "" {
		newStatus := PlaybackStatus(status)

		m.lastStateMu.RLock()
		lastStatus := m.lastState[busName]
		m.lastStateMu.RUnlock()

		if lastStatus != newStatus {
			m.lastStateMu.Lock()
			m.lastState[busName] = newStatus
			m.lastStateMu.Unlock()

			logger.Debug("[mpris] player %s changed status: %s -> %s", busName, lastStatus, newStatus)

			if newStatus == StatusPlaying {
				m.heartbeat.Start()
			}
		}
	}

	propNames := make([]string, 0, len(changed))
	for propName := range changed {
		propNames = append(propNames, propName)
	}
	logger.Debug("[mpris] updating %s properties: %v", busName, propNames)

	if err := m.UpdatePlayerProperties(busName, changed); err != nil {
		logger.Error("[mpris] failed to update player %s properties: %v", busName, err)
	}
}

func (m *MPRISBackend) handleNameOwnerChanged(sig *dbus.Signal) {
	if len(sig.Body) < 3 {
		return
	}

	busName, ok := sig.Body[0].(string)
	if !ok || !strings.HasPrefix(busName, MPRIS_PREFIX+".") {
		return
	}

	oldOwner, _ := sig.Body[1].(string)
	newOwner, _ := sig.Body[2].(string)

	if oldOwner == "" && newOwner != "" {
		logger.Info("[mpris] new player detected: %s", busName)
		if _, err := m.ReloadPlayerFromDBus(busName); err != nil {
			logger.Error("[mpris] failed to add new player %s: %v", busName, err)
		}
	} else if oldOwner != "" && newOwner == "" {
		logger.Info("[mpris] player removed: %s", busName)
		if err := m.RemovePlayer(busName); err != nil {
			logger.Error("[mpris] failed to remove player %s: %v", busName, err)
		}
	}
}
