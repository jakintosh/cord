package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"git.studiopollinator.com/pollinator/cord/internal/cli"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func renderCidr(
	t *testing.T,
	name string,
	cidr string,
) topology.Cidr {
	t.Helper()
	result, err := topology.CidrFromString(name, cidr, strings.HasSuffix(cidr, "/32"))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTopology_ReturnsWriterError(
	t *testing.T,
) {
	err := cli.New(failingWriter{}, cli.Options{}).Topology(topology.View{}, cli.TopologyOptions{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("Render() error = %v, want write failed", err)
	}
}

func TestTopology_CleansTextAndPreservesJSON(
	t *testing.T,
) {
	view := topology.View{
		EffectiveGroups: []string{"admin\nops\x1b"},
		Associations: []topology.Association{{
			Group1: "admin\nops\x1b",
			Group2: "users\tall",
		}},
	}
	opts := cli.TopologyOptions{
		Heading:  "network\nstatus\x1b",
		Metadata: "synced:\tjust now\nready",
	}
	var out bytes.Buffer
	if err := cli.New(&out, cli.Options{}).Topology(view, opts); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"network; status \n",
		"synced: just now; ready\n",
		"effective groups: [admin; ops ]\n",
		"admin; ops  <-> users all\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
	if strings.ContainsAny(out.String(), "\x1b\t\r") {
		t.Fatalf("control characters remain: %q", out.String())
	}
	out.Reset()
	if err := cli.New(&out, cli.Options{
		Format: cli.FormatJSON,
	}).Topology(view, opts); err != nil {
		t.Fatal(err)
	}
	var got topology.View
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, view) {
		t.Fatalf("JSON changed response: %#v", got)
	}
}

func TestTopology_StyleOnlyChangesANSISequences(
	t *testing.T,
) {
	view := topology.View{
		Nodes: []topology.ViewNode{{
			Cidr:    renderCidr(t, "self", "10.0.0.1/32"),
			Subject: true,
		}},
	}
	opts := cli.TopologyOptions{
		BoldSubject: true,
	}
	var plain, styled bytes.Buffer
	if err := cli.New(&plain, cli.Options{}).Topology(view, opts); err != nil {
		t.Fatal(err)
	}
	if err := cli.New(&styled, cli.Options{
		Style: true,
	}).Topology(view, opts); err != nil {
		t.Fatal(err)
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if strings.Contains(plain.String(), "\x1b") || !strings.Contains(styled.String(), "\x1b[36m◆") {
		t.Fatal("styling did not follow the CLI option")
	}
	if ansi.ReplaceAllString(styled.String(), "") != plain.String() {
		t.Fatal("styling changed text layout")
	}
}

func TestTopology_SortsAndHighlightsSubjectInPlace(
	t *testing.T,
) {
	view := topology.View{
		Nodes: []topology.ViewNode{
			{
				Cidr:          renderCidr(t, "self", "10.2.0.1/32"),
				DisplayParent: "root",
				Subject:       true,
			},
			{
				Cidr:          renderCidr(t, "early", "10.1.0.1/32"),
				DisplayParent: "root",
			},
			{
				Cidr: renderCidr(t, "root", "10.0.0.0/8"),
			},
		},
	}

	var output bytes.Buffer
	if err := cli.New(&output, cli.Options{
		Style: true,
	}).Topology(view, cli.TopologyOptions{
		BoldSubject: true,
	}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Index(text, "early") > strings.Index(text, "self") {
		t.Fatalf("subject moved ahead of ascending sibling:\n%s", text)
	}
	if !strings.Contains(text, "\x1b[1m└─ 10.2.0.1/32") {
		t.Fatalf("subject not highlighted in place:\n%s", text)
	}
}

func TestTopology_SubjectRendersFilledCyanDiamond(
	t *testing.T,
) {
	view := topology.View{
		Nodes: []topology.ViewNode{
			{
				Cidr:     renderCidr(t, "self", "10.0.0.1/32"),
				PeerName: "self",
				Subject:  true,
			},
			{
				Cidr:     renderCidr(t, "peer", "10.0.0.2/32"),
				PeerName: "peer",
			},
		},
	}

	var output bytes.Buffer
	if err := cli.New(&output, cli.Options{
		Style: true,
	}).Topology(view, cli.TopologyOptions{
		Connected: map[string]bool{
			"self": true,
			"peer": true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "10.0.0.1/32  \x1b[36m◆\x1b[39m self (you)") {
		t.Fatalf("subject diamond with cyan color not rendered:\n%s", text)
	}
	if !strings.Contains(text, "10.0.0.2/32  \x1b[32m◉\x1b[39m peer") {
		t.Fatalf("peer green status not rendered:\n%s", text)
	}
}

func TestTopology_LabelsPeerWhenNameDiffersFromCIDR(
	t *testing.T,
) {
	view := topology.View{
		Nodes: []topology.ViewNode{
			{
				Cidr:     renderCidr(t, "cord-server-cidr", "10.0.0.1/32"),
				PeerName: "cord-server",
			},
			{
				Cidr:     renderCidr(t, "alice", "10.0.0.2/32"),
				PeerName: "alice",
			},
		},
	}

	var output bytes.Buffer
	if err := cli.New(&output, cli.Options{}).Topology(view, cli.TopologyOptions{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "10.0.0.1/32  ◯ cord-server-cidr (peer: cord-server)") {
		t.Fatalf("differing peer name not rendered:\n%s", text)
	}
	if strings.Contains(text, "alice (peer: alice)") {
		t.Fatalf("matching peer name rendered redundantly:\n%s", text)
	}
}

func TestTopology_AlignedColumnsAndPeerState(
	t *testing.T,
) {
	view := topology.View{
		Nodes: []topology.ViewNode{
			{
				Cidr: renderCidr(t, "root", "10.99.0.0/16"),
			},
			{
				Cidr:          renderCidr(t, "cloud", "10.99.0.0/17"),
				DisplayParent: "root",
			},
			{
				Cidr:          renderCidr(t, "servers", "10.99.0.0/20"),
				DisplayParent: "cloud",
			},
			{
				Cidr:          renderCidr(t, "cord-server", "10.99.0.1/32"),
				DisplayParent: "servers",
				PeerName:      "cord-server",
				Groups:        []string{"cord-server"},
			},
			{
				Cidr:          renderCidr(t, "nimbus", "10.99.4.1/32"),
				DisplayParent: "servers",
				PeerName:      "nimbus",
				Groups:        []string{"admin"},
			},
			{
				Cidr:          renderCidr(t, "fleet", "10.99.128.0/17"),
				DisplayParent: "root",
			},
			{
				Cidr:          renderCidr(t, "neon", "10.99.128.1/32"),
				DisplayParent: "fleet",
				PeerName:      "neon",
				Groups:        []string{"admin"},
			},
		},
	}

	var output bytes.Buffer
	if err := cli.New(&output, cli.Options{}).Topology(view, cli.TopologyOptions{
		Connected: map[string]bool{
			"cord-server": false,
			"nimbus":      false,
			"neon":        true,
		},
	}); err != nil {
		t.Fatal(err)
	}

	text := output.String()
	for _, want := range []string{
		"CIDR",
		"NAME",
		"GROUPS",
		"│     ├─ 10.99.0.1/32  ◯ cord-server  [cord-server]",
		"│     └─ 10.99.4.1/32  ◯ nimbus       [admin]",
		"   └─ 10.99.128.1/32   ◉ neon         [admin]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}

	lines := strings.Split(text, "\n")
	column := func(line, value string) int {
		index := strings.Index(line, value)
		if index < 0 {
			return -1
		}
		return utf8.RuneCountInString(line[:index])
	}
	nameStarts := []int{
		column(lines[3], "root"),
		column(lines[6], "cord-server"),
		column(lines[7], "nimbus"),
		column(lines[8], "fleet"),
		column(lines[9], "neon"),
	}
	for _, start := range nameStarts {
		if start != nameStarts[0] {
			t.Fatalf("name columns are not aligned: %v\n%s", nameStarts, text)
		}
	}
	if column(lines[6], "◯") != column(lines[7], "◯") ||
		column(lines[7], "◯") != column(lines[9], "◉") {
		t.Fatalf("status indicators are not aligned:\n%s", text)
	}
}
