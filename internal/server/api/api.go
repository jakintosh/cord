package api

type StatusResponse struct {
	Status string `json:"status"`
}

type DeleteResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}
