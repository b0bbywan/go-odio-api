package remotedesktop

import (
    "context"
    "fmt"
    "math/rand"
    "os"
    "strings"
    "time"

    "github.com/godbus/dbus/v5"
    "github.com/b0bbywan/go-odio-api/config"
    "github.com/b0bbywan/go-odio-api/logger"
)

// New creates a new RemoteDesktop backend
func New(ctx context.Context, cfg *config.RemoteDesktopConfig) (*RemoteDesktopBackend, error) {
    if cfg == nil || !cfg.Enabled {
        return nil, nil
    }

    conn, err := dbus.ConnectSessionBus()
    if err != nil {
        return nil, err
    }

    name := conn.Names()[0]
    sender := strings.ReplaceAll(strings.TrimPrefix(name, ":"), ".", "_")

    backend := &RemoteDesktopBackend{
        conn:       conn,
        ctx:        ctx,
        timeout:    10 * time.Second,
        sender:     sender,
        portal:     conn.Object(portalDest, portalPath),
        tokenFile:  cfg.TokenFile,
    }

    if err := backend.initSession(); err != nil {
        logger.Error("[remotedesktop] failed to initialize session: %v", err)
        backend.Close()
        return nil, err
    }

    logger.Info("[remotedesktop] backend initialized")
    return backend, nil
}

// Close cleanly closes connections
func (r *RemoteDesktopBackend) Close() {
    if r.conn != nil {
        if err := r.conn.Close(); err != nil {
            logger.Error("[remotedesktop] failed to close D-Bus connection: %v", err)
        }
        r.conn = nil
    }
}

// MovePointer moves the pointer relatively by dx, dy pixels
func (r *RemoteDesktopBackend) MovePointer(dx, dy float64) error {
    return r.portal.Call(
        portalIface+".NotifyPointerMotion", 0,
        r.session,
        map[string]dbus.Variant{},
        dx, dy,
    ).Err
}

func (r *RemoteDesktopBackend) initSession() error {
    if err := r.createSession(); err != nil {
        return fmt.Errorf("CreateSession: %w", err)
    }

    restoreToken := r.loadToken()
    if err := r.selectDevices(restoreToken); err != nil {
        return fmt.Errorf("SelectDevices: %w", err)
    }

    token, err := r.start()
    if err != nil {
        return fmt.Errorf("Start: %w", err)
    }

    if token != "" {
        r.saveToken(token)
    }
    return nil
}

func (r *RemoteDesktopBackend) createSession() error {
    tok := r.token()
    sessTok := r.token()
    opts := map[string]dbus.Variant{
        "handle_token":         dbus.MakeVariant(tok),
        "session_handle_token": dbus.MakeVariant(sessTok),
    }

    var requestPath dbus.ObjectPath
    if err := r.portal.Call(portalIface+".CreateSession", 0, opts).Store(&requestPath); err != nil {
        return err
    }

    results, err := r.waitResponse(requestPath)
    if err != nil {
        return err
    }

    r.session = results["session_handle"].Value().(dbus.ObjectPath)
    return nil
}

func (r *RemoteDesktopBackend) selectDevices(restoreToken string) error {
    tok := r.token()
    opts := map[string]dbus.Variant{
        "handle_token": dbus.MakeVariant(tok),
        "types":        dbus.MakeVariant(uint32(DevicePointer)),
        "persist_mode": dbus.MakeVariant(uint32(PersistModePermanent)),
    }
    if restoreToken != "" {
        opts["restore_token"] = dbus.MakeVariant(restoreToken)
    }

    var requestPath dbus.ObjectPath
    if err := r.portal.Call(portalIface+".SelectDevices", 0, r.session, opts).Store(&requestPath); err != nil {
        return err
    }

    _, err := r.waitResponse(requestPath)
    return err
}

func (r *RemoteDesktopBackend) start() (string, error) {
    tok := r.token()
    opts := map[string]dbus.Variant{
        "handle_token": dbus.MakeVariant(tok),
    }

    var requestPath dbus.ObjectPath
    if err := r.portal.Call(portalIface+".Start", 0, r.session, "", opts).Store(&requestPath); err != nil {
        return "", err
    }

    results, err := r.waitResponse(requestPath)
    if err != nil {
        return "", err
    }

    if v, ok := results["restore_token"]; ok {
        return v.Value().(string), nil
    }
    return "", nil
}

func (r *RemoteDesktopBackend) waitResponse(requestPath dbus.ObjectPath) (map[string]dbus.Variant, error) {
    ch := make(chan *dbus.Signal, 1)
    r.conn.Signal(ch)
    defer r.conn.RemoveSignal(ch)

    r.conn.AddMatchSignal(
        dbus.WithMatchObjectPath(requestPath),
        dbus.WithMatchInterface(requestIface),
        dbus.WithMatchMember("Response"),
    )
    defer r.conn.RemoveMatchSignal(
        dbus.WithMatchObjectPath(requestPath),
        dbus.WithMatchInterface(requestIface),
        dbus.WithMatchMember("Response"),
    )

    select {
    case sig := <-ch:
        code := sig.Body[0].(uint32)
        if code != 0 {
            return nil, fmt.Errorf("portal response code: %d", code)
        }
        return sig.Body[1].(map[string]dbus.Variant), nil
    case <-time.After(r.timeout):
        return nil, fmt.Errorf("timeout waiting for portal response")
    case <-r.ctx.Done():
        return nil, r.ctx.Err()
    }
}

func (r *RemoteDesktopBackend) loadToken() string {
    data, err := os.ReadFile(r.tokenFile)
    if err != nil {
        return ""
    }
    return strings.TrimSpace(string(data))
}

func (r *RemoteDesktopBackend) saveToken(token string) {
    if err := os.WriteFile(r.tokenFile, []byte(token), 0600); err != nil {
        logger.Error("[remotedesktop] failed to save restore token: %v", err)
    }
}

func (r *RemoteDesktopBackend) token() string {
    b := make([]byte, 8)
    for i := range b {
        b[i] = 'a' + byte(rand.Intn(26))
    }
    return string(b)
}
