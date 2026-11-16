package paths

import (
	"path/filepath"
	"testing"
)

// TestStateDir checks the state directory's table: the XDG variable first, then
// the home directory, and an error when neither is set.
func TestStateDir(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// env is the environment of the call.
		env map[string]string
		// want is the directory, when the call passes.
		want string
		// fail is set when the call must fail.
		fail bool
	}{
		{
			name: "the XDG state home",
			env:  map[string]string{stateHomeEnv: "/home/example/state", homeEnv: "/home/example"},
			want: filepath.Join("/home/example/state", "frostmarks"),
		},
		{
			name: "the home directory",
			env:  map[string]string{homeEnv: "/home/example"},
			want: filepath.Join("/home/example", ".local", "state", "frostmarks"),
		},
		{
			name: "neither variable",
			env:  map[string]string{},
			fail: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(name string) string { return test.env[name] }
			got, err := StateDir(getenv)
			if test.fail {
				if err == nil {
					t.Fatalf("StateDir passed = %q, and it must fail", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("StateDir: %v", err)
			}
			if got != test.want {
				t.Errorf("StateDir = %q, want %q", got, test.want)
			}
		})
	}
}

// TestSocket checks the socket's table: the runtime directory, and an error that
// names the flag when it is unset.
func TestSocket(t *testing.T) {
	getenv := func(name string) string {
		if name == runtimeDirEnv {
			return "/run/user/1000"
		}
		return ""
	}
	got, err := Socket(getenv)
	if err != nil {
		t.Fatalf("Socket: %v", err)
	}
	if want := filepath.Join("/run/user/1000", "frostmarks.sock"); got != want {
		t.Errorf("Socket = %q, want %q", got, want)
	}

	_, err = Socket(func(string) string { return "" })
	if err == nil {
		t.Fatal("Socket passed without a runtime directory, and it must fail")
	}
	if want := "no socket path: set XDG_RUNTIME_DIR, or pass -socket"; err.Error() != want {
		t.Errorf("the error = %q, want %q", err.Error(), want)
	}
}
