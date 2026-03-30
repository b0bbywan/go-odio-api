package dbus

// Standard D-Bus method names
const (
	DBUS_INTERFACE = "org.freedesktop.DBus"

	INTROSPECTABLE     = DBUS_INTERFACE + ".Introspectable"
	BUS_LIST_NAMES     = DBUS_INTERFACE + ".ListNames"
	BUS_ADD_MATCH      = DBUS_INTERFACE + ".AddMatch"
	BUS_REMOVE_MATCH   = DBUS_INTERFACE + ".RemoveMatch"
	BUS_GET_NAME_OWNER = DBUS_INTERFACE + ".GetNameOwner"
	DBUS_PROP_IFACE    = DBUS_INTERFACE + ".Properties"
	DBUS_OBJECT_MNGR   = DBUS_INTERFACE + ".ObjectManager"

	PROP_GET     = DBUS_PROP_IFACE + ".Get"
	PROP_SET     = DBUS_PROP_IFACE + ".Set"
	PROP_GET_ALL = DBUS_PROP_IFACE + ".GetAll"

	MANAGED_OBJECTS = DBUS_OBJECT_MNGR + ".GetManagedObjects"
)
