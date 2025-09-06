package server

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
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
	cfgFile, err := ctx.Config.GetConfigWriter(fileName)
	if err != nil {
		return fmt.Errorf("failed to create config writer: %w", err)
	}

	// Generate server WireGuard keypair in server layer
	privKey, err := wg.GeneratePrivateKey()
	pubKey := privKey.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to generate wireguard keypair: %w", err)
	}

	// Create root CIDR and initial server peer atomically in the store
	if err := ctx.Store.Create(ctx.Name, cidr, pubKey.String()); err != nil {
		return fmt.Errorf("failed to create network and server peer: %w", err)
	}

	// Build device config and write it using the generated private key
	serverIp := utils.GetFirstAssignableIpFromCidr(cidr)
	deviceCfg, err := wg.NewDeviceConfig(privKey, cidr, serverIp, port)
	if err != nil {
		return fmt.Errorf("failed to build device config: %w", err)
	}

	if err := deviceCfg.Write(cfgFile); err != nil {
		return fmt.Errorf("failed to write device config file")
	}

	return nil
}

func (ctx *Context) DeleteNetwork() error {

	return ctx.Store.Delete(ctx.Name)
}
