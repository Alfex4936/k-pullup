package dto

import "github.com/Alfex4936/chulbong-kr/model"

type SignUpRequest struct {
	Email      string  `json:"email"`
	Password   string  `json:"password,omitempty"`   // Optional, not used for OAuth2 sign-ups
	Provider   string  `json:"provider,omitempty"`   // e.g., "google", "kakao", empty for traditional sign-ups
	ProviderID string  `json:"providerId,omitempty"` // Unique ID from the provider, empty for traditional sign-ups
	Username   *string `json:"username,omitempty"`   // Optional for traditional, used if provided for OAuth2
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string      `json:"token"`
	User  *model.User `json:"user"`
}

type UpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
}

// UserData holds the information extracted from the context.
type UserData struct {
	UserID   int
	Username string
}

type UserMarkers struct {
	MarkersWithPhotos []MarkerSimpleWithDescription `json:"markers"`
	CurrentPage       int                           `json:"currentPage"`
	TotalPages        int                           `json:"totalPages"`
	TotalMarkers      int                           `json:"totalMarkers"`
}

// User corresponds to the Users table in the database
type UserResponse struct {
	Username          string `json:"username" db:"Username"`
	Email             string `json:"email" db:"Email"`
	Provider          string `json:"provider,omitempty" db:"Provider"`
	ContributionLevel string `json:"contributionLevel,omitempty"`
	UserID            int    `json:"userId" db:"UserID"`
	ReportCount       int    `json:"reportCount,omitempty" db:"ReportCount"`
	MarkerCount       int    `json:"markerCount,omitempty" db:"MarkerCount"`
	ContributionCount int    `json:"contributionCount,omitempty"`
	Chulbong          bool   `json:"chulbong,omitempty"`
}
