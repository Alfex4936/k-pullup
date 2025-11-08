package model

import "time"

type Notice struct {
	NoticeID  int       `db:"NoticeID" json:"noticeId"`
	Title     string    `db:"Title" json:"title"`
	Content   string    `db:"Content" json:"content"`
	CreatedAt time.Time `db:"CreatedAt" json:"createdAt"`
	UpdatedAt time.Time `db:"UpdatedAt" json:"updatedAt"`
}
