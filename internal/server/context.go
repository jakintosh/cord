package server

import (
	"bytes"
	"database/sql"
	"fmt"
)

type Context struct {
	Name   string
	Db     *sql.DB
	Config Config
	Data   Data
}

func NewFsContext(
	network string,
	configDir string,
	dataDir string,
) (*Context, error) {

	config := &FsConfig{configDir}
	data := &FsData{dataDir}

	database, err := data.OpenDatabase(network)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx := &Context{
		Name:   network,
		Db:     database,
		Config: config,
		Data:   data,
	}

	return ctx, nil
}

func NewMemContext(
	network string,
) (*Context, error) {

	config := &MemConfig{map[string]*bytes.Buffer{}}
	data := &MemData{}

	database, err := data.OpenDatabase(network)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx := &Context{
		Name:   network,
		Db:     database,
		Config: config,
		Data:   data,
	}

	return ctx, nil

}
