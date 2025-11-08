package dto

import "time"

type ReactionCounts struct {
	ThumbsUp   int `json:"thumbsUp" db:"ThumbsUp"`
	ThumbsDown int `json:"thumbsDown" db:"ThumbsDown"`
}

type StoryResponse struct {
	Username   string    `json:"username" db:"Username"`
	Caption    string    `json:"caption" db:"Caption"`
	PhotoURL   string    `json:"photoURL" db:"PhotoURL"`
	Blurhash   string    `json:"blurhash,omitempty" db:"Blurhash"`
	Address    string    `json:"address,omitempty" db:"Address"`
	CreatedAt  time.Time `json:"createdAt" db:"CreatedAt"`
	ExpiresAt  time.Time `json:"expiresAt" db:"ExpiresAt"`
	StoryID    int       `json:"storyID" db:"StoryID"`
	MarkerID   int       `json:"markerID" db:"MarkerID"`
	UserID     int       `json:"userID" db:"UserID"`
	ThumbsUp   int       `json:"thumbsUp,omitempty" db:"ThumbsUp"`
	ThumbsDown int       `json:"thumbsDown,omitempty" db:"ThumbsDown"`
	UserLiked  *bool     `json:"userLiked,omitempty" db:"UserLiked"`
}

type ReactionRequest struct {
	ReactionType string `json:"reactionType"`
}

// Type alias for story response when querying for one marker
type StoryResponseOneMarker = StoryResponse
