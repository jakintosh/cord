package cli_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli"
	client "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
)

func TestClientTopology_PresentsSyncTimeAndPeerObservations(
	t *testing.T,
) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	connected := true
	response := client.NetworkTopology{
		SyncedAt: now.Add(-42 * time.Second),
		Nodes: []client.TopologyNode{{
			Name:      "peer",
			CIDR:      "10.0.0.1/32",
			Terminal:  true,
			PeerName:  "peer",
			Connected: &connected,
		}},
	}
	var out bytes.Buffer
	output := cli.New(&out, cli.Options{
		Clock: func() time.Time { return now },
	})
	if err := output.ClientTopology(response); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"topology (projected)", "synced: 42s ago", "◉ peer"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
}
