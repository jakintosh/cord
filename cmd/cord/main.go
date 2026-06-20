package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
)

const (
	BIN_NAME     = "cord"
	AUTHOR       = "jakintosh"
	DEFAULT_SOCK = "/tmp/cord.sock"
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
	Options: []args.Option{
		{
			Long: "socket-path",
			Type: args.OptionTypeParameter,
			Help: "path to the daemon unix socket",
		},
	},
	Subcommands: []*args.Command{
		version.Command(VersionInfo),
		daemonCmd,
		statusCmd,
	},
}
