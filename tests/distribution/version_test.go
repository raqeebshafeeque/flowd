package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionDiscovery(t *testing.T) {
	for _, test := range []struct{ name, injected, want string }{{"release", "0.1.0", "0.1.0"}, {"development", "", "dev"}} {
		t.Run(test.name, func(t *testing.T) {
			binary := buildVersion(t, test.injected)
			directory := filepath.Join(t.TempDir(), "data")
			t.Setenv("XDG_DATA_HOME", directory)
			for _, mode := range []struct {
				args []string
				want string
			}{
				{[]string{"--version"}, "flowd " + test.want + "\n"},
				{[]string{"--json", "--version"}, "{\"version\":\"" + test.want + "\"}\n"},
				{[]string{"--help"}, "Usage: flowd"},
			} {
				output, err := exec.Command(binary, mode.args...).CombinedOutput()
				if err != nil || !strings.Contains(string(output), mode.want) {
					t.Errorf("discovery %v: %v; %s", mode.args, err, output)
				}
			}
			if _, err := os.Stat(directory); !os.IsNotExist(err) {
				t.Error("version/help opened storage")
			}
		})
	}
}
