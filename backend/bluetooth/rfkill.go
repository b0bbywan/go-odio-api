package bluetooth

import (
	"github.com/amenzhinsky/rfkill"
	"github.com/b0bbywan/go-odio-api/logger"
)

// unblockIfSoftBlocked unblocks any soft-blocked Bluetooth rfkill device.
func unblockIfSoftBlocked() {
	if err := rfkill.Each(func(ev rfkill.Event) error {
		if ev.Type != rfkill.TypeBluetooth || ev.Soft == 0 {
			return nil
		}
		if err := rfkill.BlockByIdx(ev.Idx, false); err != nil {
			logger.Warn("[bluetooth] rfkill: failed to unblock device %d: %v", ev.Idx, err)
			return nil
		}
		logger.Info("[bluetooth] rfkill: unblocked soft-blocked device %d", ev.Idx)
		return nil
	}); err != nil {
		logger.Warn("[bluetooth] rfkill: failed to iterate devices: %v", err)
	}
}
