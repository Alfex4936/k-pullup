package dto

// NoticePostDTO represents a notice creation request
type NoticePostDTO struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UserWarningRequest represents a request to update user warning count
type UserWarningRequest struct {
	UserID int `json:"userId"`
	Action int `json:"action"` // 1 to increase, -1 to decrease
	Reason string `json:"reason,omitempty"`
}

// UserWarningResponse represents a user with warning information
type UserWarningResponse struct {
	UserID       int    `json:"userId" db:"UserID"`
	Username     string `json:"username" db:"Username"`
	Email        string `json:"email" db:"Email"`
	WarningCount int    `json:"warningCount" db:"WarningCount"`
}
