package login1

const (
	// D-Bus system constants
	DBUS_INTERFACE  = "org.freedesktop.DBus"
	DBUS_PROP_IFACE = DBUS_INTERFACE + ".Properties"

	LOGIN1_PREFIX    = "org.freedesktop.login1"
	LOGIN1_PATH      = "/org/freedesktop/login1"
	LOGIN1_INTERFACE = LOGIN1_PREFIX + ".Manager"

	LOGIN1_METHOD_POWEROFF = LOGIN1_INTERFACE + ".PowerOff"
	LOGIN1_METHOD_REBOOT   = LOGIN1_INTERFACE + ".Reboot"

	LOGIN1_CAPABILITY_REBOOT   = LOGIN1_INTERFACE + ".CanReboot"
	LOGIN1_CAPABILITY_POWEROFF = LOGIN1_INTERFACE + ".CanPowerOff"
)
