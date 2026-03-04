package model

import "time"

// Project is a funding project.
type Project struct {
	ID        string
	Name      string
	Code      string
	Version   int
	CreatedAt time.Time
}
