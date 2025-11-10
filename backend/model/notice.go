package model

import "time"

// Notice represents a system notice or announcement
type Notice struct {
	NoticeID  int       `json:"noticeId" db:"NoticeID"`
	Title     string    `json:"title" db:"Title"`
	Content   string    `json:"content" db:"Content"`
	CreatedAt time.Time `json:"createdAt" db:"CreatedAt"`
	UpdatedAt time.Time `json:"updatedAt" db:"UpdatedAt"`
}
