package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/cli"
	server "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

func TestServerTopology_PresentsPeerObservations(
	t *testing.T,
) {
	connected := true
	response := server.NetworkTopology{
		Nodes: []server.TopologyNode{{
			Name:      "peer",
			CIDR:      "10.0.0.1/32",
			Terminal:  true,
			PeerName:  "peer",
			Connected: &connected,
		}},
	}
	var out bytes.Buffer
	output := cli.New(&out, cli.Options{})
	if err := output.ServerTopology(response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "◉ peer") || strings.Contains(out.String(), "synced:") {
		t.Fatalf("server topology: %s", out.String())
	}
}
