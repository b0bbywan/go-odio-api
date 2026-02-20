package remotedesktop

import (
    "context"
    "time"

    "github.com/godbus/dbus/v5"
)

const (
    portalDest   = "org.freedesktop.portal.Desktop"
    portalPath   = "/org/freedesktop/portal/desktop"
    portalIface  = "org.freedesktop.portal.RemoteDesktop"
    requestIface = "org.freedesktop.portal.Request"
)

type DeviceType uint32
const (
    DeviceKeyboard   DeviceType = 1
    DevicePointer    DeviceType = 2
    DeviceTouchscreen DeviceType = 4
)

type PersistMode uint32
const (
    PersistModeNone      PersistMode = 0
    PersistModeTransient PersistMode = 1
    PersistModePermanent PersistMode = 2
)

type RemoteDesktopBackend struct {
    conn      *dbus.Conn
    ctx       context.Context
    timeout   time.Duration
    sender    string
    portal    dbus.BusObject
    session   dbus.ObjectPath
    tokenFile string
}
