package client

import (
	"database/sql"
	"fmt"
	"os"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
)

type Context struct {
	Db        *sql.DB
	Name      string
	ConfigDir string
	DataDir   string
}

func NewContext(
	network string,
	configDir string,
	dataDir string,
) (*Context, error) {

	os.MkdirAll(configDir, 0755)
	database, err := db.Open(network, dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Context{
		Db:        database,
		Name:      network,
		ConfigDir: configDir,
		DataDir:   dataDir,
	}, nil
}

func (ctx *Context) Install() error {

	fmt.Printf(
		"Install\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

func (ctx *Context) Uninstall() error {

	fmt.Printf(
		"Uninstall\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

func (ctx *Context) Show() error {

	fmt.Printf(
		"Show\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

func (ctx *Context) Fetch() error {

	fmt.Printf(
		"Fetch\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

func (ctx *Context) Up() error {

	fmt.Printf(
		"Up\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

func (ctx *Context) Down() error {

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}
