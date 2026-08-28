package text_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
	topotext "git.studiopollinator.com/pollinator/cord/internal/topology/text"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

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

func TestRender_ReturnsWriterError(t *testing.T) {
	err := topotext.Render(failingWriter{}, topology.View{}, topotext.Options{})
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("Render() error = %v, want write failed", err)
	}
}

func TestRender_SortsAndHighlightsSubjectInPlace(
	t *testing.T,
) {
	view := topology.View{Nodes: []topology.ViewNode{
		{Cidr: renderCidr(t, "self", "10.2.0.1/32"), DisplayParent: "root", Subject: true},
		{Cidr: renderCidr(t, "early", "10.1.0.1/32"), DisplayParent: "root"},
		{Cidr: renderCidr(t, "root", "10.0.0.0/8")},
	}}

	var output bytes.Buffer
	if err := topotext.Render(&output, view, topotext.Options{BoldSubject: true}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Index(text, "early") > strings.Index(text, "self") {
		t.Fatalf("subject moved ahead of ascending sibling:\n%s", text)
	}
	if !strings.Contains(text, "\x1b[1m└─ 10.2.0.1/32    self (you)") {
		t.Fatalf("subject not highlighted in place:\n%s", text)
	}
}

func TestRender_LabelsPeerWhenNameDiffersFromCIDR(t *testing.T) {
	view := topology.View{Nodes: []topology.ViewNode{
		{
			Cidr:     renderCidr(t, "cord-server-cidr", "10.0.0.1/32"),
			PeerName: "cord-server",
		},
		{
			Cidr:     renderCidr(t, "alice", "10.0.0.2/32"),
			PeerName: "alice",
		},
	}}

	var output bytes.Buffer
	if err := topotext.Render(&output, view, topotext.Options{}); err != nil {
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

func TestRender_AlignedColumnsAndPeerState(t *testing.T) {
	view := topology.View{Nodes: []topology.ViewNode{
		{Cidr: renderCidr(t, "root", "10.99.0.0/16")},
		{Cidr: renderCidr(t, "cloud", "10.99.0.0/17"), DisplayParent: "root"},
		{Cidr: renderCidr(t, "servers", "10.99.0.0/20"), DisplayParent: "cloud"},
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
		{Cidr: renderCidr(t, "fleet", "10.99.128.0/17"), DisplayParent: "root"},
		{
			Cidr:          renderCidr(t, "neon", "10.99.128.1/32"),
			DisplayParent: "fleet",
			PeerName:      "neon",
			Groups:        []string{"admin"},
		},
	}}

	var output bytes.Buffer
	if err := topotext.Render(&output, view, topotext.Options{
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
