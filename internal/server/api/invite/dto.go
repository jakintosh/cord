package invite

type RedeemInviteRequest struct {
	TempPubKey string `json:"temp_pubkey"`
	PermPubKey string `json:"perm_pubkey"`
}

type RedeemResultDTO struct {
	NetworkName  string        `json:"network_name"`
	AssignedCidr string        `json:"assigned_cidr"`
	Server       ServerInfoDTO `json:"server"`
}

type ServerInfoDTO struct {
	PublicKey        string `json:"public_key"`
	ExternalEndpoint string `json:"external_endpoint"`
	InternalEndpoint string `json:"internal_endpoint"`
}
