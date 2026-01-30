package main

import (
	"encoding/json"
	"net/http"
)

// optional code omitted

type Server struct{}

func NewServer() Server {
	return Server{}
}

// This function takes any value and returns a pointer to it
func Ptr[T any](v T) *T {
	return &v
}

// (GET /ping)
func (Server) GetNetdata(w http.ResponseWriter, r *http.Request) {
	resp := netData

	w.Header().Set("Content-Type", "application/json") // Good practice to set the header
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
