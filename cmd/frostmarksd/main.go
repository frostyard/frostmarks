// Command frostmarksd is frostmarks' daemon: the app's API on its private Unix
// socket, the app's database in its state directory, and the outbox relay that
// publishes the app's events.
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

	"github.com/frostyard/hearth/events"
	"github.com/frostyard/hearth/events/inproc"
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
	bus := inproc.New()
	cancelEvents, err := bus.Subscribe("marks.>", func(event events.Event) {
		logger.Debug("event", "type", event.Type, "subject", event.Subject, "selector", event.Selector, "topic", event.Topic)
	})
	if err != nil {
		fmt.Fprintf(stderr, "frostmarksd: subscribe to the events: %v\n", err)
		_ = db.Close()
		return exitError
	}
	defer cancelEvents()
	relay := outbox.NewRelay(db, bus, outbox.Options{Logger: logger})
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
	failure := rpc.Serve(ctx, listener, rpc.Handler(service.API()))

	// The stop is the server's, then the relay's, then the database's.
	stopRelay()
	<-relayDone
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
