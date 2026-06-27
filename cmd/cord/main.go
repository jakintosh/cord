package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
)

const (
	BIN_NAME = "cord"
	AUTHOR   = "jakintosh"
)

func main() {
	root.Parse()
}

var root = &args.Command{
	Name: BIN_NAME,
	Help: "coordinate wireguard networks",
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
		serverCmd,
		clientCmd,
	},
}
