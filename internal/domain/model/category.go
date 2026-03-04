package model

import "time"

// Category provides structured classification for transactions.
type Category struct {
	ID        string
	UserID    string
	Name      string
	Type      string  // 'income' | 'expense' | 'transfer'
	ParentID  *string // optional second level
	SortOrder int
	IsActive  bool
	Version   int
	CreatedAt time.Time
}
