package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	topotext "git.studiopollinator.com/pollinator/cord/internal/topology/text"
)

const (
	BIN_NAME = "cordtopo"
	AUTHOR   = "jakintosh"
)

var root = &args.Command{
	Name: BIN_NAME,
	Help: "inspect and test cidr-group topology",
	Config: &args.Config{
		Author:  AUTHOR,
		Version: VersionInfo.Version,
		HelpOption: &args.HelpOption{
			Short: 'h',
			Long:  "help",
		},
	},
	Subcommands: []*args.Command{
		version.Command(VersionInfo),
		{
			Name: "full",
			Help: "show the complete topology",
			Operands: []args.Operand{
				{
					Name: "config",
					Help: "path to the topology config JSON",
				},
			},
			Handler: func(i *args.Input) error {
				configPath := i.GetOperand("config")
				snap, _, db, err := setup(configPath)
				if err != nil {
					return err
				}
				defer db.Close()
				topo, err := topology.New(snap)
				if err != nil {
					return fmt.Errorf("compile topology: %w", err)
				}
				return topotext.Render(os.Stdout, topo.FullView(), topotext.Options{})
			},
		},
		{
			Name: "visible",
			Help: "show the visible topology for a peer",
			Operands: []args.Operand{
				{
					Name: "config",
					Help: "path to the topology config JSON",
				},
				{
					Name: "peer",
					Help: "peer name",
				},
			},
			Handler: func(i *args.Input) error {
				configPath := i.GetOperand("config")
				peerName := i.GetOperand("peer")

				snap, _, db, err := setup(configPath)
				if err != nil {
					return err
				}
				defer db.Close()

				topo, err := topology.New(snap)
				if err != nil {
					return fmt.Errorf("compile topology: %w", err)
				}
				view, err := topo.ProjectedView(peerName)
				if err != nil {
					return err
				}
				return topotext.Render(os.Stdout, view, topotext.Options{
					Heading: "topology (projected)",
				})
			},
		},
		{
			Name: "explain",
			Help: "explain the visibility between two peers",
			Operands: []args.Operand{
				{
					Name: "config",
					Help: "path to the topology config JSON",
				},
				{
					Name: "peer1",
					Help: "first peer name",
				},
				{
					Name: "peer2",
					Help: "second peer name",
				},
			},
			Handler: func(i *args.Input) error {
				configPath := i.GetOperand("config")
				peer1 := i.GetOperand("peer1")
				peer2 := i.GetOperand("peer2")

				snap, cfg, db, err := setup(configPath)
				if err != nil {
					return err
				}
				defer db.Close()

				topo, err := topology.New(snap)
				if err != nil {
					return fmt.Errorf("compile topology: %w", err)
				}

				resolver := topo.Resolver()

				cidr1, ok := resolver.GetPeerCIDR(peer1)
				if !ok {
					return err
				}
				cidr2, ok := resolver.GetPeerCIDR(peer2)
				if !ok {
					return err
				}
				eff1, err := resolver.GetEffectiveGroups(cidr1)
				if err != nil {
					return err
				}
				eff2, err := resolver.GetEffectiveGroups(cidr2)
				if err != nil {
					return err
				}

				renderHeading(fmt.Sprintf("topology (for %s)", peer1))
				renderPeerAncestryAndGroups(os.Stdout, cidr1, eff1, topo.Tree())

				fmt.Println()
				renderHeading(fmt.Sprintf("topology (for %s)", peer2))
				renderPeerAncestryAndGroups(os.Stdout, cidr2, eff2, topo.Tree())

				fmt.Println()
				renderHeading("association paths")
				visible := renderConnectingPaths(cfg.Associations, eff1, eff2)

				if visible {
					fmt.Println("\nVERDICT: visible")
				} else {
					fmt.Println("\nVERDICT: not visible")
				}
				return nil
			},
		},
		{
			Name: "test",
			Help: "run expected-visibility tests",
			Operands: []args.Operand{
				{
					Name: "config",
					Help: "path to the topology config JSON",
				},
			},
			Handler: func(i *args.Input) error {
				configPath := i.GetOperand("config")

				snap, cfg, db, err := setup(configPath)
				if err != nil {
					return err
				}
				defer db.Close()

				resolver, err := topology.NewResolver(snap)
				if err != nil {
					return fmt.Errorf("compile topology: %w", err)
				}

				failures := 0
				for peerName, expected := range cfg.Expected {
					visible, err := resolver.VisiblePeers(peerName)
					if err != nil {
						fmt.Printf("  FAIL %s: %v\n", peerName, err)
						failures++
						continue
					}
					got := make([]string, len(visible))
					for i, p := range visible {
						got[i] = p.Name
					}
					sort.Strings(got)
					sort.Strings(expected)

					gotSet := make(map[string]bool)
					for _, n := range got {
						gotSet[n] = true
					}
					expSet := make(map[string]bool)
					for _, n := range expected {
						expSet[n] = true
					}

					match := true
					for _, n := range got {
						if !expSet[n] {
							match = false
							break
						}
					}
					for _, n := range expected {
						if !gotSet[n] {
							match = false
							break
						}
					}

					if match {
						fmt.Printf(
							"  PASS %s: [%s]\n",
							peerName,
							strings.Join(got, ", "),
						)
					} else {
						fmt.Printf(
							"  FAIL %s: got [%s], want [%s]\n",
							peerName,
							strings.Join(got, ", "),
							strings.Join(expected, ", "),
						)
						failures++
					}
				}

				if failures > 0 {
					return fmt.Errorf("%d failures", failures)
				}
				fmt.Println("\nall passed")
				return nil
			},
		},
	},
}

func main() {
	root.Parse()
}
