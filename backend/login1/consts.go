package login1

const (
	// D-Bus system constants
	DBUS_INTERFACE  = "org.freedesktop.DBus"
	DBUS_PROP_IFACE = DBUS_INTERFACE + ".Properties"

	// D-Bus method names
	DBUS_LIST_NAMES_METHOD   = DBUS_INTERFACE + ".ListNames"
	DBUS_ADD_MATCH_METHOD    = DBUS_INTERFACE + ".AddMatch"
	DBUS_PROP_GET            = DBUS_PROP_IFACE + ".Get"
	DBUS_PROP_GET_ALL        = DBUS_PROP_IFACE + ".GetAll"
	DBUS_PROP_SET            = DBUS_PROP_IFACE + ".Set"
	DBUS_PROP_CHANGED_SIGNAL = DBUS_PROP_IFACE + ".PropertiesChanged"
	DBUS_NAME_OWNER_CHANGED  = DBUS_INTERFACE + ".NameOwnerChanged"
	DBUS_GET_NAME_OWNER      = DBUS_INTERFACE + ".GetNameOwner"

	LOGIN1_PREFIX    = "org.freedesktop.login1"
	LOGIN1_PATH      = "/org/freedesktop/login1"
	LOGIN1_INTERFACE = LOGIN1_PREFIX + ".Manager"

	LOGIN1_METHOD_POWEROFF = LOGIN1_INTERFACE + ".PowerOff"
	LOGIN1_METHOD_REBOOT   = LOGIN1_INTERFACE + ".Reboot"

	LOGIN1_CAPABILITY_REBOOT   = LOGIN1_INTERFACE + ".CanReboot"
	LOGIN1_CAPABILITY_POWEROFF = LOGIN1_INTERFACE + ".CanPowerOff"
)
