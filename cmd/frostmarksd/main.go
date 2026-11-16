// Command frostmarksd is frostmarks' daemon: the app's API on its private Unix
// socket, the app's database in its state directory, and the outbox relay that
// publishes the app's events.
//
// With -lodge it also serves the app's lodge socket, on which lodge forwards a
// user's calls, and it publishes the app's events to the broker that lodge
// embeds (docs/specs/lodge-app.md).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/frostyard/hearth/events/outbox"

	"github.com/frostyard/frostmarks/internal/paths"
	"github.com/frostyard/frostmarks/internal/rpc"
	"github.com/frostyard/frostmarks/marks"
)

// version is the build's version, which -ldflags sets.
var version = "dev"

// The daemon's exit codes.
const (
	// exitOK is a clean stop.
	exitOK = 0
	// exitError is an error of the daemon.
	exitError = 1
	// exitUsage is a usage error: an unknown flag, or an argument.
	exitUsage = 2
)

// main runs the daemon and exits with its code.
func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr))
}

// run runs the daemon and returns its exit code: exitOK for a clean stop on
// SIGINT or SIGTERM, exitError for an error, and exitUsage for a usage error.
func run(args []string, getenv func(string) string, stderr io.Writer) int {
	flags := flag.NewFlagSet("frostmarksd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state-dir", "", "the state directory (default $XDG_STATE_HOME/frostmarks)")
	socketPath := flags.String("socket", "", "the private socket (default $XDG_RUNTIME_DIR/frostmarks.sock)")
	lodge := flags.Bool("lodge", false, "serve lodge's app socket and publish the events to lodge's broker")
	debug := flags.Bool("debug", false, "log each event at debug level")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "frostmarksd: unexpected argument %q\n", flags.Arg(0))
		flags.Usage()
		return exitUsage
	}
	if *showVersion {
		fmt.Fprintf(stderr, "frostmarksd %s\n", version)
		return exitOK
	}
	dir := *stateDir
	if dir == "" {
		defaultDir, err := paths.StateDir(getenv)
		if err != nil {
			fmt.Fprintf(stderr, "frostmarksd: %v\n", err)
			return exitError
		}
		dir = defaultDir
	}
	socket := *socketPath
	if socket == "" {
		defaultSocket, err := paths.Socket(getenv)
		if err != nil {
			fmt.Fprintf(stderr, "frostmarksd: %v\n", err)
			return exitError
		}
		socket = defaultSocket
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "frostmarksd: make the state directory %s: %v\n", dir, err)
		return exitError
	}
	db, err := marks.Open(ctx, filepath.Join(dir, "frostmarks.db"))
	if err != nil {
		fmt.Fprintf(stderr, "frostmarksd: %v\n", err)
		return exitError
	}
	appEvents, err := newDaemonEvents(*lodge, getenv, logger)
	if err != nil {
		fmt.Fprintf(stderr, "frostmarksd: %v\n", err)
		_ = db.Close()
		return exitError
	}
	// The transport is closed when the relay has stopped, below, and a stop
	// before that closes it here: a close is safe twice.
	defer appEvents.close()
	relay := outbox.NewRelay(db, appEvents.transport, outbox.Options{Logger: logger})
	service := &marks.Service{DB: db, Notify: relay.Notify}
	listener, err := rpc.Listen(socket)
	if err != nil {
		fmt.Fprintf(stderr, "frostmarksd: %v\n", err)
		_ = db.Close()
		return exitError
	}

	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		// Run returns nil when its context ends: a failed drain is the next
		// wake's work, and it is logged.
		_ = relay.Run(relayCtx)
	}()

	fmt.Fprintf(stderr, "frostmarksd: serving on %s\n", socket)
	// The 2 servers run together, and the first of them to end stops the other.
	serveCtx, stopServe := context.WithCancel(ctx)
	defer stopServe()
	servers := 1
	failed := make(chan error, 2)
	go func() {
		failed <- rpc.Serve(serveCtx, listener, rpc.Handler(service.API()))
	}()
	if appEvents.socket != nil {
		servers = 2
		if appEvents.path == "" {
			fmt.Fprintln(stderr, "frostmarksd: serving lodge's socket from socket activation")
		} else {
			fmt.Fprintf(stderr, "frostmarksd: serving lodge's socket on %s\n", appEvents.path)
		}
		health := func(ctx context.Context) rpc.Health {
			return rpc.Health{Version: version, Protocol: 1, Ping: db.PingContext}
		}
		go func() {
			failed <- rpc.ServeLodge(serveCtx, appEvents.socket, rpc.LodgeHandler(service.API(), health))
		}()
	}
	var failure error
	for range servers {
		if err := <-failed; err != nil && failure == nil {
			failure = err
		}
		// The stop of 1 server is the stop of the other: a failed server ends
		// the daemon, and a signal ends both.
		stopServe()
	}

	// The stop is the servers', then the relay's, then the broker's, then the
	// database's.
	stopRelay()
	<-relayDone
	appEvents.close()
	if err := db.Close(); err != nil && failure == nil {
		failure = fmt.Errorf("close the database: %w", err)
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, fs.ErrNotExist) && failure == nil {
		failure = fmt.Errorf("remove the socket %s: %w", socket, err)
	}
	if failure != nil {
		fmt.Fprintf(stderr, "frostmarksd: %v\n", failure)
		return exitError
	}
	logger.Info("stopped")
	return exitOK
}
