package feed

import "time"

type Health struct {
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
	Warning   string    `json:"warning,omitempty"`
}

func (h Health) Healthy() bool {
	return h.Error == "" && h.Warning == ""
}
