package mpris

import (
	"github.com/godbus/dbus/v5"
)

// newPlayer crée un nouveau Player avec connexion D-Bus
func newPlayer(conn *dbus.Conn, busName string) *Player {
	return &Player{
		conn:    conn,
		BusName: busName,
	}
}

