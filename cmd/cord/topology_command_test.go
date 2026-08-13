package main

import "testing"

func TestNetworkBranchesIncludeTopologyCommands(
	t *testing.T,
) {
	for branch, command := range map[string]string{
		"server": "topology",
		"client": "topology",
	} {
		var found bool
		var subcommands = serverNetworkCmd.Subcommands
		if branch == "client" {
			subcommands = clientNetworkCmd.Subcommands
		}
		for _, subcommand := range subcommands {
			if subcommand.Name == command {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s network branch is missing %s command", branch, command)
		}
	}
}
