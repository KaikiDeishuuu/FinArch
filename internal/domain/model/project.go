package model

import "time"

// Project is a funding project.
type Project struct {
	ID        string
	Name      string
	Code      string
	CreatedAt time.Time
}
