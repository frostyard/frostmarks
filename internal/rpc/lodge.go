// This file serves the app's lodge socket, on which lodge forwards a user's calls
// after it has checked that user's grant for this app (docs/specs/lodge-app.md,
// and lodge's docs/specs/app-contract.md).
//
// The socket admits lodge's user alone, and the Lodge-Principal and Lodge-Grant
// headers are read only through hearth's adapter (lodge.Handler), after that
// check. The app's own /health answer stands outside the adapter: lodge polls it
// for the rail, and it carries no grant.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/frostyard/hearth/connectserver"
	hearthlodge "github.com/frostyard/hearth/lodge"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
)

// The lodge socket's health path and its limits.
const (
	// healthPath is the app's health, which lodge polls.
	healthPath = "/health"
	// protocolMajor is the major of the app's own API: its RPC major, which
	// /health answers.
	protocolMajor = 1
	// lodgeSocketPerm is the permission of the lodge socket: its own user, and
	// group lodge.
	lodgeSocketPerm = 0o660
	// lodgeDirPerm is the permission of the lodge socket's directories.
	lodgeDirPerm = 0o755
)

// Health is the app's state, as GET /health answers it (lodge's
// docs/specs/app-contract.md, What an app serves on its socket).
type Health struct {
	// Version is the app's version.
	Version string
	// Protocol is the major of the app's own API.
	Protocol int
	// Ping checks the app's store. A store that does not answer gives
	// {"ok": false, "reason": ...}.
	Ping func(context.Context) error
}

// healthAnswer is 1 answer of GET /health, in the order of its members.
type healthAnswer struct {
	// OK is set when the app serves.
	OK bool `json:"ok"`
	// Version is the app's version.
	Version string `json:"version,omitempty"`
	// Protocol is the major of the app's own API.
	Protocol int `json:"protocol,omitempty"`
	// Reason says why the app is not ok.
	Reason string `json:"reason,omitempty"`
}

// LodgeHandler returns the HTTP handler of the app's lodge socket: GET and HEAD
// /health for lodge's polls, and the Connect API at /connect/ for a caller whose
// kind is principal. Every other path, and a kind that the path does not admit,
// is 403 with 1 line of text.
//
// health is a function, so that each poll answers with the current version of
// the app and pings the store of the moment.
func LodgeHandler(api marksv1.MarksAPI, health func(context.Context) Health) http.Handler {
	path, h := marksv1connect.NewMarksHandler(api, connectserver.HandlerOptions()...)
	// The adapter reads Lodge-Principal and Lodge-Grant, and only after the
	// peer check of the socket's listener marked the connection.
	connectHandler := connectserver.Lodge(path, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == healthPath:
			serveHealth(w, r, health)
		case strings.HasPrefix(r.URL.Path, connectserver.LodgePrefix):
			connectHandler.ServeHTTP(w, r)
		default:
			refuse(w, "a path outside the app contract admits no caller")
		}
	})
}

// serveHealth answers the app's state, and it admits GET and HEAD alone.
func serveHealth(w http.ResponseWriter, r *http.Request, health func(context.Context) Health) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "the health path admits GET and HEAD only", http.StatusMethodNotAllowed)
		return
	}
	state := health(r.Context())
	answer := healthAnswer{OK: true, Version: state.Version, Protocol: state.Protocol}
	if answer.Protocol == 0 {
		answer.Protocol = protocolMajor
	}
	if state.Ping != nil {
		if err := state.Ping(r.Context()); err != nil {
			answer = healthAnswer{OK: false, Reason: oneLine(err.Error())}
		}
	}
	body, err := json.Marshal(answer)
	if err != nil {
		http.Error(w, "the health answer does not encode", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(body, '\n'))
}

// refuse answers 1 request with 403 and 1 line of text.
func refuse(w http.ResponseWriter, reason string) {
	http.Error(w, "forbidden: "+oneLine(reason), http.StatusForbidden)
}

// oneLine returns 1 text on 1 line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// ServeLodge serves h on the lodge socket's listener until ctx ends, and it
// returns nil after a clean stop. A call in flight has 5 seconds to finish.
//
// The server marks every connection that hearth's lodge listener accepted in the
// connection's context, which the adapter requires: without the mark it refuses
// every request.
func ServeLodge(ctx context.Context, listener net.Listener, h http.Handler) error {
	if err := connectserver.Serve(ctx, listener, h); err != nil {
		return fmt.Errorf("the lodge socket: %w", err)
	}
	return nil
}

// ListenLodge listens on the lodge socket at path, and it serves lodge's user
// alone: another peer's connection is closed before the server reads a byte of
// it.
//
// It makes the socket's directory (0755) when it is missing, it clears a stale
// socket as the private socket does, and it sets the socket's mode to 0660. The
// socket unit owns these when systemd passes the socket instead.
func ListenLodge(path string, lodgeUID int) (net.Listener, error) {
	if dir := parentDir(path); dir != "" {
		if err := makeDirMode(dir, lodgeDirPerm); err != nil {
			return nil, err
		}
	}
	if err := clearPath(path, dialTimeoutSocket); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := chmod(path, lodgeSocketPerm); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return hearthlodge.Listener(listener, lodgeUID), nil
}
