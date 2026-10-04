package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli"
	client "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
	server "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

func TestCLI_UsesItsWriterAndClock(
	t *testing.T,
) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	handshake := now.Add(-42 * time.Second).Format(time.RFC3339)
	var first, second bytes.Buffer
	one := cli.New(&first, cli.Options{
		Clock: func() time.Time { return now },
	})
	two := cli.New(&second, cli.Options{})
	if err := one.ClientPeers([]client.Peer{{
		Name:          "peer",
		LastHandshake: &handshake,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := two.Groups([]server.Group{{
		Name: "admin",
	}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), "42s ago") || strings.Contains(first.String(), "admin") {
		t.Fatalf("first destination: %q", first.String())
	}
	if second.String() != "NAME\nadmin\n" {
		t.Fatalf("second destination: %q", second.String())
	}
}

func TestCLI_MessageIsSilentOnlyInJSONMode(
	t *testing.T,
) {
	for _, format := range []cli.Format{cli.FormatText, cli.FormatJSON} {
		var out bytes.Buffer
		output := cli.New(&out, cli.Options{
			Format: format,
		})
		if err := output.Message("network %q deleted\n", "example"); err != nil {
			t.Fatal(err)
		}
		want := "network \"example\" deleted\n"
		if format == cli.FormatJSON {
			want = ""
		}
		if out.String() != want {
			t.Fatalf("format %v: output %q, want %q", format, out.String(), want)
		}
	}
	// Silent success must not touch the destination at all.
	if err := cli.New(failingWriter{}, cli.Options{
		Format: cli.FormatJSON,
	}).Message("ok\n"); err != nil {
		t.Fatalf("silent confirmation wrote to destination: %v", err)
	}
}

func TestCLI_JSONPresentersPreserveResponses(
	t *testing.T,
) {
	clientStatus := client.Status{
		Version: "dev",
		Health:  "degraded",
	}
	serverStatus := server.Status{
		Version: "dev",
		Health:  "healthy",
	}
	clientNetwork := client.Network{
		Name: "example",
	}
	serverNetwork := server.Network{
		Name: "example",
	}
	clientRuntime := client.NetworkStatus{
		Name:   "example",
		Health: "degraded",
	}
	serverRuntime := server.NetworkStatus{
		Name:   "example",
		Health: "healthy",
	}
	clientTopology := client.NetworkTopology{
		SubjectPeer: "self",
	}
	serverTopology := server.NetworkTopology{
		SubjectPeer: "self",
	}
	groups := []server.Group{{
		Name: "admin",
	}}
	for _, tc := range []struct {
		name     string
		response any
		write    func(*cli.CLI) error
	}{
		{
			name:     "client status",
			response: clientStatus,
			write:    func(c *cli.CLI) error { return c.ClientStatus(clientStatus) },
		},
		{
			name:     "server status",
			response: serverStatus,
			write:    func(c *cli.CLI) error { return c.ServerStatus(serverStatus) },
		},
		{
			name:     "client network",
			response: clientNetwork,
			write:    func(c *cli.CLI) error { return c.ClientNetworkDetail(clientNetwork) },
		},
		{
			name:     "server network",
			response: serverNetwork,
			write:    func(c *cli.CLI) error { return c.ServerNetworkDetail(serverNetwork) },
		},
		{
			name:     "client runtime",
			response: clientRuntime,
			write:    func(c *cli.CLI) error { return c.ClientNetworkRuntime(clientRuntime) },
		},
		{
			name:     "server runtime",
			response: serverRuntime,
			write:    func(c *cli.CLI) error { return c.ServerNetworkRuntime(serverRuntime) },
		},
		{
			name:     "client topology",
			response: clientTopology,
			write:    func(c *cli.CLI) error { return c.ClientTopology(clientTopology) },
		},
		{
			name:     "server topology",
			response: serverTopology,
			write:    func(c *cli.CLI) error { return c.ServerTopology(serverTopology) },
		},
		{
			name:     "groups",
			response: groups,
			write:    func(c *cli.CLI) error { return c.Groups(groups) },
		},
		{
			name:     "mutation response",
			response: map[string]string{},
			write:    func(c *cli.CLI) error { return c.Result(map[string]string{}, "created\n") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			output := cli.New(&out, cli.Options{
				Format:  cli.FormatJSON,
				Style:   true,
				Verbose: true,
			})
			if err := tc.write(output); err != nil {
				t.Fatal(err)
			}
			want, err := json.MarshalIndent(tc.response, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != string(want)+"\n" {
				t.Fatalf("output=%q, want=%s", got, want)
			}
		})
	}
}

func TestCLI_JSONEmptyListsAreArrays(
	t *testing.T,
) {
	for name, write := range map[string]func(*cli.CLI) error{
		"names":         func(c *cli.CLI) error { return c.Names(nil) },
		"groups":        func(c *cli.CLI) error { return c.Groups(nil) },
		"cidrs":         func(c *cli.CLI) error { return c.Cidrs(nil) },
		"associations":  func(c *cli.CLI) error { return c.Associations(nil) },
		"registrations": func(c *cli.CLI) error { return c.Registrations(nil) },
		"client peers":  func(c *cli.CLI) error { return c.ClientPeers(nil) },
		"server peers":  func(c *cli.CLI) error { return c.ServerPeers(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := write(cli.New(&out, cli.Options{
				Format: cli.FormatJSON,
			})); err != nil {
				t.Fatal(err)
			}
			if out.String() != "[]\n" {
				t.Fatalf("empty list=%q", out.String())
			}
		})
	}
}

func TestCLI_ReturnsOutputErrors(
	t *testing.T,
) {
	output := cli.New(failingWriter{}, cli.Options{})
	for name, write := range map[string]func() error{
		"json": func() error { return output.JSON(map[string]string{"name": "example"}) },
		"list": func() error { return output.Groups(nil) },
		"detail": func() error {
			return output.ClientNetworkDetail(client.Network{
				Name: "example",
			})
		},
		"health": func() error {
			return output.ServerNetworkRuntime(server.NetworkStatus{
				Health: "degraded",
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := write(); !errors.Is(err, errWrite) {
				t.Fatal("destination error was ignored")
			}
		})
	}
}

func TestCLI_JSONPreservesIndentedResponse(
	t *testing.T,
) {
	var out bytes.Buffer
	output := cli.New(&out, cli.Options{})
	if err := output.JSON(map[string]string{"name": "example"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "{\n  \"name\": \"example\"\n}\n" {
		t.Fatalf("JSON = %q", got)
	}
}

var errWrite = errors.New("write failed")

func TestCLI_CleansTextValuesAndPreservesJSON(
	t *testing.T,
) {
	name := "office\nlab\x1b"
	reason := "failed\r\nretry\tsoon"
	clientNetwork := client.Network{
		Name:           name,
		State:          name,
		Address:        name,
		Interface:      name,
		ServerEndpoint: name,
	}
	serverNetwork := server.Network{
		Name:       name,
		ExternalIP: name,
		MainName:   name,
		MainCidr:   name,
		InviteName: name,
		InviteCidr: name,
	}
	clientStatus := client.NetworkStatus{
		Health: "degraded",
		Reason: reason,
	}
	serverStatus := server.NetworkStatus{
		Health: "degraded",
		Reason: reason,
	}
	for _, tc := range []struct {
		name     string
		response any
		write    func(*cli.CLI) error
		want     string
	}{
		{
			name:     "names",
			response: []string{name},
			write:    func(c *cli.CLI) error { return c.Names([]string{name}) },
			want:     "office; lab \n",
		},
		{
			name:     "client detail",
			response: clientNetwork,
			write:    func(c *cli.CLI) error { return c.ClientNetworkDetail(clientNetwork) },
			want:     "name: office; lab \nstate: office; lab \nenabled: false\naddress: office; lab \ninterface: office; lab \nserver_endpoint: office; lab \n",
		},
		{
			name:     "server detail",
			response: serverNetwork,
			write:    func(c *cli.CLI) error { return c.ServerNetworkDetail(serverNetwork) },
			want:     "name: office; lab \nexternal_ip: office; lab \nmain_name: office; lab \nmain_cidr: office; lab \nmain_wg_port: 0\nmain_api_port: 0\ninvite_name: office; lab \ninvite_cidr: office; lab \ninvite_wg_port: 0\ninvite_api_port: 0\nenabled: false\n",
		},
		{
			name:     "client health",
			response: clientStatus,
			write:    func(c *cli.CLI) error { return c.ClientNetworkRuntime(clientStatus) },
			want:     "health: degraded (failed ; retry soon)\n",
		},
		{
			name:     "server health",
			response: serverStatus,
			write:    func(c *cli.CLI) error { return c.ServerNetworkRuntime(serverStatus) },
			want:     "health: degraded (failed ; retry soon)\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.write(cli.New(&out, cli.Options{})); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("text = %q, want %q", out.String(), tc.want)
			}
			out.Reset()
			if err := tc.write(cli.New(&out, cli.Options{
				Format: cli.FormatJSON,
			})); err != nil {
				t.Fatal(err)
			}
			want, err := json.MarshalIndent(tc.response, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if out.String() != string(want)+"\n" {
				t.Fatalf("JSON changed response: %s", out.String())
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write(
	[]byte,
) (
	int,
	error,
) {
	return 0, errWrite
}
