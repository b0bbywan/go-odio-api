package bluetooth

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestNewDBusListener(t *testing.T) {
	ctx := context.Background()
	callback := func(sig *dbus.Signal) bool { return false }
	matchRules := []string{
		"type='signal',interface='org.freedesktop.DBus.Properties',arg0='org.bluez.Device1'",
		"type='signal',interface='org.freedesktop.DBus.Properties',arg0='org.bluez.Adapter1'",
	}

	listener := NewDBusListener(nil, ctx, matchRules, callback)

	if listener.ctx != ctx {
		t.Error("context not set correctly")
	}
	if len(listener.matchRules) != len(matchRules) {
		t.Fatalf("matchRules len = %d, want %d", len(listener.matchRules), len(matchRules))
	}
	for i, rule := range matchRules {
		if listener.matchRules[i] != rule {
			t.Errorf("matchRules[%d] = %q, want %q", i, listener.matchRules[i], rule)
		}
	}
	if listener.callback == nil {
		t.Error("callback not set")
	}
	if listener.signals == nil {
		t.Error("signals channel not created")
	}
	if cap(listener.signals) != 10 {
		t.Errorf("signals channel capacity = %d, want 10", cap(listener.signals))
	}
}

func TestListenStopsOnCallbackTrue(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	callback := func(sig *dbus.Signal) bool {
		callCount++
		return true
	}

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	listener.signals <- &dbus.Signal{Path: "/test/device"}

	done := make(chan struct{})
	go func() {
		listener.Listen()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(time.Second):
		t.Fatal("Listen did not stop after callback returned true")
	}

	if callCount != 1 {
		t.Errorf("callback called %d times, want 1", callCount)
	}
}

func TestListenContinuesOnCallbackFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCount := 0
	callback := func(sig *dbus.Signal) bool {
		callCount++
		if callCount >= 3 {
			cancel()
		}
		return false
	}

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	for i := 0; i < 3; i++ {
		listener.signals <- &dbus.Signal{Path: dbus.ObjectPath("/test/device")}
	}

	done := make(chan struct{})
	go func() {
		listener.Listen()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(time.Second):
		t.Fatal("Listen did not stop after context cancellation")
	}

	if callCount != 3 {
		t.Errorf("callback called %d times, want 3", callCount)
	}
}

func TestListenStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	callback := func(sig *dbus.Signal) bool {
		callCount++
		return false
	}

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	done := make(chan struct{})
	go func() {
		listener.Listen()
		close(done)
	}()

	// Cancel before any signal is sent
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(time.Second):
		t.Fatal("Listen did not stop after context cancellation")
	}

	if callCount != 0 {
		t.Errorf("callback called %d times, want 0", callCount)
	}
}

func TestListenStopsOnContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callback := func(sig *dbus.Signal) bool {
		return false
	}

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	done := make(chan struct{})
	go func() {
		listener.Listen()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(time.Second):
		t.Fatal("Listen did not stop after context timeout")
	}
}

func TestListenProcessesSignalsInOrder(t *testing.T) {
	ctx := context.Background()

	received := []dbus.ObjectPath{}
	callback := func(sig *dbus.Signal) bool {
		received = append(received, sig.Path)
		return sig.Path == "/stop"
	}

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	listener.signals <- &dbus.Signal{Path: "/dev/1"}
	listener.signals <- &dbus.Signal{Path: "/dev/2"}
	listener.signals <- &dbus.Signal{Path: "/stop"}
	listener.signals <- &dbus.Signal{Path: "/should/not/reach"}

	done := make(chan struct{})
	go func() {
		listener.Listen()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(time.Second):
		t.Fatal("Listen did not stop")
	}

	expected := []dbus.ObjectPath{"/dev/1", "/dev/2", "/stop"}
	if len(received) != len(expected) {
		t.Fatalf("received %d signals, want %d", len(received), len(expected))
	}
	for i, p := range expected {
		if received[i] != p {
			t.Errorf("signal %d path = %q, want %q", i, received[i], p)
		}
	}
}

func TestStopClosesChannelAndRemovesSignal(t *testing.T) {
	ctx := context.Background()
	callback := func(sig *dbus.Signal) bool { return false }

	listener := &DBusListener{
		ctx:      ctx,
		callback: callback,
		signals:  make(chan *dbus.Signal, 10),
	}

	listener.Stop()

	// Verify channel is closed - reading twice
	// First read might get buffered value, second confirms closed
	<-listener.signals
	_, ok := <-listener.signals
	if ok {
		t.Error("channel should be closed after Stop()")
	}
}
