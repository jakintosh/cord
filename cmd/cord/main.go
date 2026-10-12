package main

import (
	"os"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
	"git.studiopollinator.com/pollinator/cord/internal/cli"
	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
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
	Options: []args.Option{{
		Long:  "verbose",
		Short: 'v',
		Type:  args.OptionTypeFlag,
		Help:  "show additional presentation details where available",
	}},
	Subcommands: []*args.Command{
		version.Command(VersionInfo),
		serverCmd,
		clientCmd,
	},
}

// cliApp constructs the presentation layer for the current command.
func cliApp(
	i *args.Input,
) *cli.CLI {
	format := cli.FormatText
	if i.GetFlag("json") {
		format = cli.FormatJSON
	}
	return cli.New(os.Stdout, cli.Options{
		Format:  format,
		Style:   render.StyleEnabled(os.Stdout),
		Verbose: i.GetFlag("verbose"),
	})
}
