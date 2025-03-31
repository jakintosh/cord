package server

import (
	"fmt"
	"net"
	"os"
	"path"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	"git.sr.ht/~jakintosh/innernet-go/internal/utils"
)

func (ctx *Context) CreateNetwork(
	cidr *net.IPNet,
	address net.IP,
	port uint16,
) error {

	if err := utils.ValidateHostName(ctx.Name); err != nil {
		return fmt.Errorf("failed to validate network name: %w", err)
	}

	// make sure we get file handle before we do all the db work
	fileName := ctx.Name + ".toml"
	savePath := path.Join(ctx.ConfigDir, fileName)
	cfgFile, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", savePath, err)
	}

	if err := initServerDb(ctx.Db); err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	if err := ctx.CreateRootCidr(cidr); err != nil {
		return fmt.Errorf("failed to add root cidr: %w", err)
	}

	serverIp := utils.GetFirstAssignableIpFromCidr(cidr)
	pubKey, peerCfg, err := ctx.CreatePeer("innernet-server", serverIp, true, 0)
	if err != nil {
		return fmt.Errorf("failed to add server peer: %w", err)
	}

	if err := ctx.RedeemPeer(pubKey.String(), pubKey.String()); err != nil {
		return fmt.Errorf("failed to redeem server peer: %w", err)
	}

	// TODO: also write out the server config file here

	err = peerCfg.WriteConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (ctx *Context) DeleteNetwork() error {

	// a network is really just a sqlite database file
	if err := db.Delete(ctx.Name, ctx.DataDir); err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}
	return nil
}
