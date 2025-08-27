package server

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
)

type Config interface {
	GetConfigWriter(name string) (io.Writer, error)
}

// FsConfig
// Uses the filesystem to manage the configuration

type FsConfig struct {
	Directory string
}

func NewFsConfig(dir string) *FsConfig {
	return &FsConfig{dir}
}

func (c *FsConfig) GetConfigWriter(name string) (io.Writer, error) {

	filepath := path.Join(c.Directory, name)
	w, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file '%s': %w", filepath, err)
	}
	return w, nil
}

// MemConfig
// Uses memory to manage the configuration

type MemConfig struct {
	Buffers map[string]*bytes.Buffer
}

func NewMemConfig() *MemConfig {
	return &MemConfig{
		Buffers: map[string]*bytes.Buffer{},
	}
}

func (c *MemConfig) GetConfigWriter(name string) (io.Writer, error) {

	if buf, ok := c.Buffers[name]; ok {
		return buf, nil
	} else {
		c.Buffers[name] = &bytes.Buffer{}
		return c.Buffers[name], nil
	}
}
