package service

type WG interface {
	GenerateKey() (string, error)
	PublicKey(privateKey string) (string, error)
	NewDevice(name, privateKey, address string, port uint16) (WGDevice, error)
	RemoveDevice(name string) error
}

type WGDevice interface {
	ApplyPeers(peers []WGPeer) error
	Up() error
	Down(remove bool) error
	DeviceName() string
}

type WGPeer struct {
	PublicKey  string
	AllowedIPs []string
	Endpoint   string
}
