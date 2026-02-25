package dto

import "time"

type Source struct {
	ID           string
	Type         string
	Name         string
	ParentID     string
	DestParentID string
	Path         string
}

type Event struct {
	ID        string
	Type      string
	UserID    int64
	Source    *Source
	CreatedAt time.Time
}
