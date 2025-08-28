package api

import (
	"github.com/gorilla/mux"
	"net/http"
)

func (a *API) handlePostConfirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["key"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	// Placeholder: respond success
	writeData(w, http.StatusOK, nil)
}
