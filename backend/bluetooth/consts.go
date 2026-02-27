package bluetooth

const (
	BLUETOOTH_PREFIX  = "org.bluez"
	BLUETOOTH_ADAPTER = BLUETOOTH_PREFIX + ".Adapter1"
	BLUETOOTH_DEVICE  = BLUETOOTH_PREFIX + ".Device1"

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

	CapDisplayOnly     = "DisplayOnly"
	CapDisplayYesNo    = "DisplayYesNo"
	CapKeyboardOnly    = "KeyboardOnly"
	CapNoInputNoOutput = "NoInputNoOutput"
	CapKeyboardDisplay = "KeyboardDisplay"

	BT_PROP_ADAPTER = "Adapter"
	BT_PROP_ADDRESS = "Address"
	BT_PROP_NAME    = "Name"
)

type BluetoothState string

const (
	BT_STATE_CONNECTED    BluetoothState = "Connected"
	BT_STATE_DISCOVERABLE BluetoothState = "Discoverable"
	BT_STATE_PAIRABLE     BluetoothState = "Pairable"
	BT_STATE_PAIRED       BluetoothState = "Paired"
	BT_STATE_POWERED      BluetoothState = "Powered"
	BT_STATE_TRUSTED      BluetoothState = "Trusted"
)

func (b BluetoothState) String() string {
	return string(b)
}
