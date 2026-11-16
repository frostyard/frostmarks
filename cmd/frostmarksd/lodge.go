// This file is the daemon's lodge side, which -lodge turns on: the app's lodge
// socket, the user whose connections it admits, and the transport that publishes
// the app's events to the broker that lodge embeds (docs/specs/lodge-app.md, and
// lodge's docs/specs/app-contract.md).
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/frostyard/hearth/events"
	"github.com/frostyard/hearth/events/broker"
	"github.com/frostyard/hearth/events/inproc"
	hearthlodge "github.com/frostyard/hearth/lodge"

	"github.com/frostyard/frostmarks/internal/eventbus"
	"github.com/frostyard/frostmarks/internal/rpc"
)

// The names and the paths of lodge's app contract.
const (
	// lodgeRootEnv is the environment variable that overrides every lodge
	// path, for development and tests. Unset, or "", is the real root.
	lodgeRootEnv = "LODGE_ROOT"
	// lodgeUIDEnv is the environment variable that names lodge's user id
	// under a root. Unset is the daemon's own id.
	lodgeUIDEnv = "LODGE_UID"
	// lodgeSocketName is the name of the app's lodge socket: the socket unit's
	// FileDescriptorName, and the name the daemon asks systemd for.
	lodgeSocketName = "marks"
	// lodgeUserName is the system user that lodge runs as.
	lodgeUserName = "lodge"
	// lodgeSocketRel is the app's lodge socket under the root.
	lodgeSocketRel = "run/lodge/apps/marks.sock"
	// brokerSocketRel is the broker's socket under the root.
	brokerSocketRel = "run/lodge/nats.sock"
	// appID is the app's id, which names it to lodge and on the broker.
	appID = "marks"
	// daemonName is the daemon's name on the broker's connection.
	daemonName = "frostmarksd"
)

// daemonEvents is where the daemon's events go, and the lodge socket that it
// serves: the in-process bus, or lodge's broker.
type daemonEvents struct {
	// transport publishes the app's events.
	transport events.Transport
	// close closes the transport. A close is safe twice.
	close func()
	// socket is the lodge socket, or nil when the daemon serves none.
	socket net.Listener
	// path is the lodge socket's own path, or "" for a socket that systemd
	// passed and for a daemon without -lodge.
	path string
}

// newDaemonEvents returns where the app's events go, and the app's lodge socket
// when -lodge gives it 1.
//
// Without -lodge, the events go to the in-process bus, whose subscriber logs each
// 1 at debug level. With it, they go to lodge's broker, which the transport dials
// when it has the first event: a broker that is not there yet stops neither the
// daemon nor its API.
func newDaemonEvents(serveLodge bool, getenv func(string) string, logger *slog.Logger) (*daemonEvents, error) {
	if !serveLodge {
		bus := inproc.New()
		cancel, err := bus.Subscribe("marks.>", func(event events.Event) {
			logger.Debug("event", "type", event.Type, "subject", event.Subject, "selector", event.Selector, "topic", event.Topic)
		})
		if err != nil {
			return nil, fmt.Errorf("subscribe to the events: %w", err)
		}
		return &daemonEvents{transport: bus, close: cancel}, nil
	}
	root := getenv(lodgeRootEnv)
	uid, err := lodgeUser(root, getenv)
	if err != nil {
		return nil, err
	}
	socket, path, err := openLodgeSocket(root, uid, getenv)
	if err != nil {
		return nil, err
	}
	transport := eventbus.New(eventbusOptions(root, logger))
	return &daemonEvents{transport: transport, close: transport.Close, socket: socket, path: path}, nil
}

// eventbusOptions returns the options of the transport to lodge's broker under
// root.
func eventbusOptions(root string, logger *slog.Logger) broker.Options {
	return broker.Options{Socket: lodgePath(root, brokerSocketRel), User: appID, Name: daemonName, Logger: logger}
}

// lodgePath returns the path of 1 of lodge's files under root. An empty root is
// the real root, so the path starts with "/".
func lodgePath(root, rel string) string {
	if root == "" {
		return "/" + rel
	}
	return filepath.Join(root, rel)
}

// lodgeUser returns the user id whose connections the app's lodge socket admits:
// under a root, LODGE_UID when it is set, else the daemon's own id; without a
// root, the id of the system user lodge.
func lodgeUser(root string, getenv func(string) string) (int, error) {
	if root != "" {
		value := getenv(lodgeUIDEnv)
		if value == "" {
			return os.Getuid(), nil
		}
		uid, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("the %s %q is no decimal user id", lodgeUIDEnv, value)
		}
		return uid, nil
	}
	account, err := user.Lookup(lodgeUserName)
	if err != nil {
		return 0, fmt.Errorf("look up the system user %s: %w", lodgeUserName, err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, fmt.Errorf("the system user %s has the id %q, which is no decimal user id", lodgeUserName, account.Uid)
	}
	return uid, nil
}

// openLodgeSocket returns the app's lodge socket, which admits the connections of
// uid alone, and the path that the daemon serves: the socket that systemd passed,
// or the app's own socket under root, whose directory and mode it makes.
//
// The path is "" for a socket that systemd passed: the socket unit owns that path.
// Without a root and without a passed socket, the daemon has no way to know the
// app's socket path, so it is an error that names the socket unit.
func openLodgeSocket(root string, uid int, getenv func(string) string) (net.Listener, string, error) {
	passed, ok, err := rpc.ListenActivated(lodgeSocketName, getenv)
	if err != nil {
		return nil, "", err
	}
	if ok {
		return hearthlodge.Listener(passed, uid), "", nil
	}
	if root == "" {
		return nil, "", errors.New("no lodge socket: start frostmarksd by its socket unit frostmarks.socket, or set " + lodgeRootEnv)
	}
	path := lodgePath(root, lodgeSocketRel)
	listener, err := rpc.ListenLodge(path, uid)
	if err != nil {
		return nil, "", err
	}
	return listener, path, nil
}
