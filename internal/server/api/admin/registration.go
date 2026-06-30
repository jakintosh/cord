package admin

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type RegistrationDTO struct {
	Name      string `json:"name"`
	Redeemed  bool   `json:"redeemed"`
	ExpiresAt string `json:"expires_at"`
}

func RegistrationDTOFromService(
	reg service.Registration,
) RegistrationDTO {
	return RegistrationDTO{
		Name:      reg.Name,
		Redeemed:  reg.Redeemed,
		ExpiresAt: reg.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func RegistrationDTOsFromService(
	regs []*service.Registration,
) []RegistrationDTO {
	if regs == nil {
		return []RegistrationDTO{}
	}
	result := make([]RegistrationDTO, len(regs))
	for i, reg := range regs {
		result[i] = RegistrationDTOFromService(*reg)
	}
	return result
}

func (a *API) handleRegistrationList(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	regs, err := a.service.ListRegistrations(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, RegistrationDTOsFromService(regs))
}

func (c *Client) ListRegistrations(
	ctx context.Context,
	network string,
) (
	[]RegistrationDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/registrations")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]RegistrationDTO](resp)
}
