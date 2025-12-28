package api

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
	resp := NetData{
		Name:     Ptr("test"),
		Country:  Ptr("AT"),
		Distance: Ptr(float32(32)),
		Dlspeed:  Ptr(float32(300.02)),
		Jitter:   Ptr(float32(0.03)),
		Latency:  Ptr(float32(12)),
		Userip:   Ptr("1.2.3.4"),
		Userisp:  Ptr("Stadtwerke Schwaz"),
	}

	w.Header().Set("Content-Type", "application/json") // Good practice to set the header
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
