package bluetooth

const (
	BLUETOOTH_PREFIX  = "org.bluez"
	BLUETOOTH_ADAPTER = BLUETOOTH_PREFIX + ".Adapter1"
	BLUETOOTH_DEVICE  = BLUETOOTH_PREFIX + ".Device1"

	DBUS_INTERFACE  = "org.freedesktop.DBus"
	DBUS_PROP_IFACE = DBUS_INTERFACE + ".Properties"
	DBUS_PROP_SET   = DBUS_PROP_IFACE + ".Set"
	DBUS_PROP_GET   = DBUS_PROP_IFACE + ".Get"
	MANAGED_OBJECTS = DBUS_INTERFACE + ".ObjectManager.GetManagedObjects"

	AGENT_IFACE   = BLUETOOTH_PREFIX + ".Agent1"
	AGENT_MANAGER = BLUETOOTH_PREFIX + ".AgentManager1"

	REGISTER_AGENT   = AGENT_MANAGER + ".RegisterAgent"
	REQUEST_AGENT    = AGENT_MANAGER + ".RequestDefaultAgent"
	UNREGISTER_AGENT = AGENT_MANAGER + ".UnregisterAgent"

	BLUEZ_PATH     = "/org/bluez"
	BLUETOOTH_PATH = BLUEZ_PATH + "/hci0"
	AGENT_PATH     = BLUEZ_PATH + "/go_odio_agent"

	AGENT_CAPABILITY     = "NoInputNoOutput"
	DISCOVERABLE_TIMEOUT = "DiscoverableTimeout"
	PAIRABLE_TIMEOUT     = "PairableTimeout"
)

type BluetoothState string

const (
	BT_STATE_POWERED      BluetoothState = "Powered"
	BT_STATE_DISCOVERABLE BluetoothState = "Discoverable"
	BT_STATE_PAIRABLE     BluetoothState = "Pairable"
	BT_STATE_TRUSTED      BluetoothState = "Trusted"
)

func (b BluetoothState) toString() string {
	return string(b)
}
