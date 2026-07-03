package invite

type RedeemInviteRequest struct {
	PermPubKey string `json:"perm_pubkey"`
}

type InvitationDTO struct {
	Network NetworkInfoDTO  `json:"network"`
	Peer    PeerIdentityDTO `json:"peer"`
}

type NetworkInfoDTO struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`
	ServerRoute string `json:"server_route"`
	APIPort     uint16 `json:"api_port"`
}

type PeerIdentityDTO struct {
	CIDR       string `json:"cidr"`
	PrivateKey string `json:"private_key,omitempty"`
}
