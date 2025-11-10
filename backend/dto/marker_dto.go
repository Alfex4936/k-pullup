package dto

import (
	"time"

	"github.com/Alfex4936/chulbong-kr/model"
)

type MarkerRequest struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	MarkerID    int     `json:"markerId,omitempty"`
	Description string  `json:"description"`
	PhotoURL    string  `json:"photoUrl,omitempty"`
}

type MarkerResponse struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	MarkerID    int     `json:"markerId"`
	Description string  `json:"description"`
	// Username    string   `json:"username"`
	// UserID      int      `json:"userId"`
	// PhotoURLs   []string `json:"photoUrls"`
}

type QueryParams struct {
	Latitude  float64 `query:"latitude"`
	Longitude float64 `query:"longitude"`
	Distance  int     `query:"distance"`
	PageSize  int     `query:"n"`
	Page      int     `query:"page"`
}

type MarkerWithDistance struct {
	MarkerSimple
	Distance    float64 `json:"distance" db:"distance"` // Distance in meters
	Description string  `json:"description" db:"Description"`
	Address     *string `json:"address,omitempty" db:"Address"`
}

type MarkerWithDistanceAndPhoto struct {
	Latitude    float64 `db:"Latitude" json:"latitude"`
	Longitude   float64 `db:"Longitude" json:"longitude"`
	Distance    float64 `db:"Distance" json:"distance"`
	MarkerID    int     `db:"MarkerID" json:"markerId"`
	Description string  `db:"Description" json:"description"`
	Address     string  `db:"Address" json:"address"`
	Thumbnail   *string `db:"Thumbnail" json:"thumbnail,omitempty"`
}

type MarkerWithDislike struct {
	model.Marker
	DislikeCount int    `db:"DislikeCount"`
	Username     string `db:"Username"`
}

type MarkerSimpleSlice []MarkerSimple

// go:generate msgp
type MarkerSimple struct {
	Latitude  float64 `json:"latitude" db:"Latitude"`
	Longitude float64 `json:"longitude" db:"Longitude"`
	MarkerID  int     `json:"markerId" db:"MarkerID"`
	HasPhoto  bool    `json:"hasPhoto,omitempty" db:"HasPhoto"`
}

type MarkerNewResponse struct {
	Latitude  float64 `json:"latitude" db:"Latitude"`
	Longitude float64 `json:"longitude" db:"Longitude"`
	Address   string  `json:"address,omitempty" db:"Address"`
	HasPhoto  bool    `json:"hasPhoto,omitempty" db:"HasPhoto"`
	Username  string  `json:"username,omitempty" db:"Username"`
	MarkerID  int     `json:"markerId" db:"MarkerID"`
	UserID    int     `json:"userId,omitempty" db:"UserID"`
}

type MarkerSimpleWithDescription struct {
	Latitude    float64   `json:"latitude" db:"Latitude"`
	Longitude   float64   `json:"longitude" db:"Longitude"`
	MarkerID    int       `json:"markerId" db:"MarkerID"`
	CreatedAt   time.Time `json:"-" db:"CreatedAt"`
	Description string    `json:"description" db:"Description"`
	Address     string    `json:"address,omitempty" db:"Address"`
}

type MarkerSimpleWithAddr struct {
	Latitude  float64 `json:"latitude" db:"Latitude"`
	Longitude float64 `json:"longitude" db:"Longitude"`
	Address   string  `json:"address,omitempty" db:"Address"`
	MarkerID  int     `json:"markerId" db:"MarkerID"`
}

type MarkerOnlyWithAddr struct {
	Address  string `json:"address,omitempty" db:"Address"`
	MarkerID int    `json:"markerId" db:"MarkerID"`
}

type MarkerRSS struct {
	UpdatedAt time.Time `json:"updatedAt" db:"UpdatedAt"`
	MarkerID  int       `json:"markerId" db:"MarkerID"`
	Address   string    `json:"address,omitempty" db:"Address"`
}

type FacilityQuantity struct {
	FacilityID int `json:"facilityId"`
	Quantity   int `json:"quantity"`
}

type FacilityRequest struct {
	MarkerID   int                `json:"markerId"`
	Facilities []FacilityQuantity `json:"facilities"`
}

type MarkerRank struct {
	Clicks   int
	MarkerID string
}

type MarkerGroup struct {
	CentralMarker MarkerSimple         // 중심 마커
	NearbyMarkers []MarkerWithDistance // 중심 마커 주변의 마커들
}

type WaterAPIResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Water     bool    `json:"water"`
}

type MarkerNewPicture struct {
	PhotoURL string `json:"photoURL" db:"PhotoURL"`
	MarkerID int    `json:"markerId" db:"MarkerID"`
}

type MarkerNewPictureExtra struct {
	PhotoURL  string  `json:"photoURL" db:"PhotoURL"`
	Address   string  `json:"address,omitempty" db:"Address"`
	Weather   string  `json:"weather"`
	Latitude  float64 `json:"latitude" db:"Latitude"`
	Longitude float64 `json:"longitude" db:"Longitude"`
	MarkerID  int     `json:"markerId" db:"MarkerID"`
}

type MarkersWithUsernames struct {
	model.Marker
	Username      string `db:"Username"`
	DislikeCount  int    `db:"DislikeCount"`
	FavoriteCount int    `db:"FavoriteCount"`
}

type MarkersClose struct {
	Markers      []MarkerWithDistanceAndPhoto `json:"markers"`
	CurrentPage  int                          `json:"currentPage"`
	TotalPages   int                          `json:"totalPages"`
	TotalMarkers int                          `json:"totalMarkers"`
}

type MarkersKakaoBot struct {
	Latitude  float64 `json:"latitude" db:"Latitude"`
	Longitude float64 `json:"longitude" db:"Longitude"`
	Address   string  `json:"address,omitempty" db:"Address"`
	Username  string  `json:"username,omitempty" db:"Username"`
	MarkerID  int     `json:"markerId" db:"MarkerID"`
	UserID    int     `json:"userId,omitempty" db:"UserID"`
}
