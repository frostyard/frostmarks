// This test is the lodge app end to end: the daemon with -lodge, the broker that
// lodge embeds, and a call as lodge forwards 1 user's.
package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/frostyard/hearth/events"
)

// The names and the waits of the test.
const (
	// lodgeRootEnv is the environment variable of lodge's development root.
	lodgeRootEnv = "LODGE_ROOT"
	// lodgeUIDEnv is the environment variable of lodge's user id under a root.
	lodgeUIDEnv = "LODGE_UID"
	// principalHeader is the header that carries the caller's principal.
	principalHeader = "Lodge-Principal"
	// grantHeader is the header that carries the caller's grant.
	grantHeader = "Lodge-Grant"
	// callerHeader is the header that carries the caller kind.
	callerHeader = "Lodge-Caller"
	// principalCaller is the caller kind of a user's call.
	principalCaller = "principal"
	// createFolderPath is the path of the CreateFolder RPC.
	createFolderPath = "/connect/frostyard.frostmarks.v1.Marks/CreateFolder"
	// createBookmarkPath is the path of the CreateBookmark RPC.
	createBookmarkPath = "/connect/frostyard.frostmarks.v1.Marks/CreateBookmark"
	// createdType is the type of the event of a new bookmark.
	createdType = "org.frostyard.marks.bookmark.created"
	// waitTimeout bounds each wait of the test.
	waitTimeout = 20 * time.Second
	// fetchWait is how long 1 read of the stream waits for a message.
	fetchWait = 2 * time.Second
)

// writeGrant is the grant of a caller that may write every folder.
const writeGrant = `[{"cap":"marks:write","sel":{"folder":"/"}}]`

// TestLodge runs the daemon with -lodge over a test broker, and it checks the app
// socket, the event of a new bookmark, and the standalone daemon's own events.
func TestLodge(t *testing.T) {
	root := moduleRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	build(t, root, binDir)
	ctlPath := filepath.Join(binDir, "marksctl")
	lodgeRoot := shortDir(t)
	brokerSocket := filepath.Join(lodgeRoot, "run/lodge/nats.sock")
	broker := startBroker(t, brokerSocket)
	appSocket := filepath.Join(lodgeRoot, "run/lodge/apps/marks.sock")
	socket := filepath.Join(shortDir(t), "s.sock")

	daemon := startDaemonWith(t, startOptions{
		binary: filepath.Join(binDir, "frostmarksd"),
		args:   []string{"-lodge", "-state-dir", filepath.Join(t.TempDir(), "state"), "-socket", socket},
		env:    []string{lodgeRootEnv + "=" + lodgeRoot, lodgeUIDEnv + "=" + strconv.Itoa(os.Getuid())},
		ready:  "frostmarksd: serving lodge's socket on " + appSocket,
	})

	t.Run("health answers the app's state", func(t *testing.T) {
		status, body := lodgeCall(t, appSocket, http.MethodGet, "/health", nil)
		if status != http.StatusOK {
			t.Fatalf("GET /health = %d, want 200: %s", status, body)
		}
		var answer struct {
			// OK is set while the app serves.
			OK bool `json:"ok"`
			// Version is the app's version.
			Version string `json:"version"`
			// Protocol is the major of the app's API.
			Protocol int `json:"protocol"`
		}
		if err := json.Unmarshal(body, &answer); err != nil {
			t.Fatalf("read the health answer: %v: %s", err, body)
		}
		if !answer.OK || answer.Version != "dev" || answer.Protocol != 1 {
			t.Errorf("GET /health = %s, want ok, the version dev and protocol 1", body)
		}
	})

	var bookmarkID string
	t.Run("a user's call creates a folder and a bookmark", func(t *testing.T) {
		status, body := lodgeCall(t, appSocket, http.MethodPost, createFolderPath, []byte(`{"path":"/work"}`))
		if status != http.StatusOK {
			t.Fatalf("CreateFolder(/work) = %d, want 200: %s", status, body)
		}
		status, body = lodgeCall(t, appSocket, http.MethodPost, createBookmarkPath,
			[]byte(`{"url":"https://example.com/a","title":"A page","folder":"/work"}`))
		if status != http.StatusOK {
			t.Fatalf("CreateBookmark = %d, want 200: %s", status, body)
		}
		var saved struct {
			// Bookmark is the saved bookmark.
			Bookmark struct {
				// ID is the bookmark's id.
				ID string `json:"id"`
				// Folder is the bookmark's folder.
				Folder string `json:"folder"`
			} `json:"bookmark"`
		}
		if err := json.Unmarshal(body, &saved); err != nil {
			t.Fatalf("read the bookmark: %v: %s", err, body)
		}
		bookmarkID = saved.Bookmark.ID
		if bookmarkID == "" {
			t.Fatalf("the saved bookmark has no id: %s", body)
		}
		if got := saved.Bookmark.Folder; got != "/work" {
			t.Errorf("the bookmark's folder = %q, want /work", got)
		}
	})

	t.Run("the event reaches the broker", func(t *testing.T) {
		if bookmarkID == "" {
			t.Fatal("the call before this 1 saved no bookmark")
		}
		event := waitEvent(t, broker, createdType)
		if got := event.Author; got != "chani" {
			t.Errorf("the event's author = %q, want chani", got)
		}
		if got := event.Subject; got != bookmarkID {
			t.Errorf("the event's subject = %q, want the bookmark's id %q", got, bookmarkID)
		}
		if got := event.Source; got != "/marks" {
			t.Errorf("the event's source = %q, want /marks", got)
		}
	})

	// The daemon stops on SIGTERM, with exit code 0.
	daemon.signal(t, syscall.SIGTERM)
	if code := daemon.wait(t); code != 0 {
		t.Fatalf("the daemon exited with %d, want 0", code)
	}

	t.Run("the standalone daemon publishes in process", func(t *testing.T) {
		// The standalone daemon keeps the same root, whose broker serves: an
		// event that it published would reach the app's stream.
		before := broker.count(t)
		standalone := startDaemonWith(t, startOptions{
			binary: filepath.Join(binDir, "frostmarksd"),
			args:   []string{"-debug", "-state-dir", filepath.Join(t.TempDir(), "state"), "-socket", socket},
			env:    []string{lodgeRootEnv + "=" + lodgeRoot, lodgeUIDEnv + "=" + strconv.Itoa(os.Getuid())},
			ready:  "frostmarksd: serving on " + socket,
		})
		createFolderOf(t, ctlPath, socket, "/alone")
		createBookmarkOf(t, ctlPath, socket, "/alone")
		waitLine(t, standalone.lines, "type="+createdType)

		standalone.signal(t, syscall.SIGTERM)
		if code := standalone.wait(t); code != 0 {
			t.Fatalf("the standalone daemon exited with %d, want 0", code)
		}
		if after := broker.count(t); after != before {
			t.Errorf("the app's stream holds %d messages after the standalone daemon, want %d: it published to the broker", after, before)
		}
		if _, err := os.Stat(appSocket); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the standalone daemon made the lodge socket %s, and it serves no lodge socket", appSocket)
		}
		if _, err := os.Stat(brokerSocket); err != nil {
			t.Errorf("the broker's socket %s is gone: %v", brokerSocket, err)
		}
	})
}

// TestLodgeWithoutBroker runs the daemon with -lodge where no broker listens, and
// it checks that the app still serves its API: the outbox keeps each event until
// a drain publishes it.
func TestLodgeWithoutBroker(t *testing.T) {
	root := moduleRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	build(t, root, binDir)
	lodgeRoot := shortDir(t)
	appSocket := filepath.Join(lodgeRoot, "run/lodge/apps/marks.sock")
	socket := filepath.Join(shortDir(t), "s.sock")
	daemon := startDaemonWith(t, startOptions{
		binary: filepath.Join(binDir, "frostmarksd"),
		args:   []string{"-lodge", "-state-dir", filepath.Join(t.TempDir(), "state"), "-socket", socket},
		env:    []string{lodgeRootEnv + "=" + lodgeRoot, lodgeUIDEnv + "=" + strconv.Itoa(os.Getuid())},
		ready:  "frostmarksd: serving lodge's socket on " + appSocket,
	})
	defer func() {
		daemon.signal(t, syscall.SIGTERM)
		if code := daemon.wait(t); code != 0 {
			t.Errorf("the daemon exited with %d, want 0", code)
		}
	}()

	status, body := lodgeCall(t, appSocket, http.MethodGet, "/health", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200: %s", status, body)
	}
	status, body = lodgeCall(t, appSocket, http.MethodPost, createFolderPath, []byte(`{"path":"/work"}`))
	if status != http.StatusOK {
		t.Fatalf("CreateFolder(/work) without a broker = %d, want 200: %s", status, body)
	}
	if _, err := os.Stat(filepath.Join(lodgeRoot, "run/lodge/nats.sock")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the daemon made a broker socket, and the broker is lodge's: %v", err)
	}
}

// lodgeCall sends 1 call to the app's lodge socket, as lodge forwards 1 user's,
// and it returns the answer's status and its body.
func lodgeCall(t *testing.T, socket, method, path string, body []byte) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, "http://lodge"+path, reader)
	if err != nil {
		t.Fatalf("make the request %s %s: %v", method, path, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(callerHeader, principalCaller)
	request.Header.Set(principalHeader, "chani")
	request.Header.Set(grantHeader, base64.RawURLEncoding.EncodeToString([]byte(writeGrant)))
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close the answer of %s %s: %v", method, path, err)
		}
	}()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer of %s %s: %v", method, path, err)
	}
	return response.StatusCode, answer
}

// waitEvent reads the app's stream until it holds 1 event of the type given, and
// it returns that event. Each read waits fetchWait on the broker, and the whole
// waits waitTimeout.
func waitEvent(t *testing.T, broker *broker, eventType string) events.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), waitTimeout)
	defer cancel()
	stream, err := broker.js.Stream(ctx, streamName)
	if err != nil {
		t.Fatalf("read the app's stream: %v", err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: streamSubject,
	})
	if err != nil {
		t.Fatalf("read the app's stream: %v", err)
	}
	for {
		// The wait is the broker's: Fetch holds the call until a message
		// arrives, or until its own wait ends with none. The loop's bound is
		// the test's deadline.
		batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(fetchWait))
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("the app's stream holds no %s within %s", eventType, waitTimeout)
			}
			t.Fatalf("fetch the app's stream: %v", err)
		}
		for message := range batch.Messages() {
			event, err := events.Parse(message.Data())
			if err != nil {
				t.Fatalf("read the event on %s: %v", message.Subject(), err)
			}
			event.Topic = message.Subject()
			if event.Type == eventType {
				return event
			}
			if err := message.Ack(); err != nil {
				t.Fatalf("acknowledge the event %s: %v", event.ID, err)
			}
		}
		if ctx.Err() != nil {
			t.Fatalf("the app's stream holds no %s within %s", eventType, waitTimeout)
		}
	}
}

// createFolderOf runs create-folder with the CLI, and it fails the test when the
// call fails.
func createFolderOf(t *testing.T, ctlPath, socket, path string) {
	t.Helper()
	if _, stderr, code := ctl(t, ctlPath, socket, "create-folder", path); code != 0 {
		t.Fatalf("create-folder %s: exit %d, stderr %q", path, code, stderr)
	}
}

// createBookmarkOf runs create-bookmark with the CLI, and it fails the test when
// the call fails.
func createBookmarkOf(t *testing.T, ctlPath, socket, folder string) {
	t.Helper()
	_, stderr, code := ctl(t, ctlPath, socket, "create-bookmark",
		"-url", "https://example.com/alone", "-title", "An alone page", "-folder", folder)
	if code != 0 {
		t.Fatalf("create-bookmark in %s: exit %d, stderr %q", folder, code, stderr)
	}
}
