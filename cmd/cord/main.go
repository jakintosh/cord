package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
)

const (
	BIN_NAME = "cord"
	AUTHOR   = "jakintosh"

	CLIENT_DEFAULT_CFG  = "/etc/cord"
	CLIENT_DEFAULT_DATA = "/var/lib/cord"
	SERVER_DEFAULT_CFG  = "/etc/cord-server"
	SERVER_DEFAULT_DATA = "/var/lib/cord-server"
)

func main() {
	root.Parse()
}

var root = &args.Command{
	Name: BIN_NAME,
	Help: "map cords to wireguard interfaces",
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
		clientCmd,
		serverCmd,
	},
	Options: []args.Option{
		{
			Short: 'v',
			Long:  "verbose",
			Type:  args.OptionTypeFlag,
			Help:  "enable verbose output",
		},
		{
			Long: "config-dir",
			Type: args.OptionTypeParameter,
			Help: "directory for config files",
		},
		{
			Long: "data-dir",
			Type: args.OptionTypeParameter,
			Help: "directory for program data",
		},
	},
}
