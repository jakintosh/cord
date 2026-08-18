package text_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

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
	if !strings.Contains(text, "\x1b[1m10.2.0.1/32 => self (you)\x1b[0m") {
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
	if !strings.Contains(text, "10.0.0.1/32 => cord-server-cidr (peer: cord-server)\n") {
		t.Fatalf("differing peer name not rendered:\n%s", text)
	}
	if strings.Contains(text, "alice (peer: alice)") {
		t.Fatalf("matching peer name rendered redundantly:\n%s", text)
	}
}
