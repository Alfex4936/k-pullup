package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/jmoiron/sqlx"
)

const (
	insertDislikeQuery = "INSERT INTO MarkerDislikes (MarkerID, UserID) VALUES (?, ?) ON DUPLICATE KEY UPDATE DislikedAt=VALUES(DislikedAt)"
	deleteDislikeQuery = "DELETE FROM MarkerDislikes WHERE UserID = ? AND MarkerID = ?"
	checkDislikeQuery  = "SELECT EXISTS(SELECT 1 FROM MarkerDislikes WHERE UserID = ? AND MarkerID = ?)"
	checkFavQuery      = "SELECT EXISTS(SELECT 1 FROM Favorites WHERE UserID = ? AND MarkerID = ?)"
	// access_type: ref, query_cost: 0.95
	countFavQuery         = "SELECT COUNT(*) FROM Favorites WHERE UserID = ?"
	insertFavQuery        = "INSERT INTO Favorites (UserID, MarkerID) VALUES (?, ?)"
	checkMarkerOwnerQuery = "SELECT UserID FROM Markers WHERE MarkerID = ?"
	deleteFavQuery        = "DELETE FROM Favorites WHERE UserID = ? AND MarkerID = ?"

	getMarkersAfterIDQuery = "SELECT ST_X(Location) AS Latitude, ST_Y(Location) AS Longitude, Address, MarkerID, COALESCE(U.Username, '알 수 없는 사용자') AS Username, M.UserID FROM Markers M LEFT JOIN Users U ON M.UserID = U.UserID WHERE MarkerID > ? ORDER BY MarkerID ASC"
)

type MarkerInteractService struct {
	DB           *sqlx.DB
	CacheService *MarkerCacheService
}

func NewMarkerInteractService(db *sqlx.DB, cache *MarkerCacheService) *MarkerInteractService {
	return &MarkerInteractService{
		DB:           db,
		CacheService: cache,
	}
}

// LeaveDislike user's dislike for a marker
func (s *MarkerInteractService) LeaveDislike(userID int, markerID int) error {
	_, err := s.DB.Exec(
		insertDislikeQuery,
		markerID, userID,
	)
	if err != nil {
		return fmt.Errorf("inserting dislike: %w", err)
	}
	return nil
}

// UndoDislike undo user's dislike for a marker
func (s *MarkerInteractService) UndoDislike(userID int, markerID int) error {
	result, err := s.DB.Exec(
		deleteDislikeQuery,
		userID, markerID,
	)
	if err != nil {
		return fmt.Errorf("deleting dislike: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking affected rows: %w", err)
	}
	if rowsAffected == 0 {
		return errors.New("no dislike found to undo")
	}

	return nil
}

// This service function checks if the given user has disliked the specified marker.
func (s *MarkerInteractService) CheckUserDislike(userID, markerID int) (bool, error) {
	var exists bool
	err := s.DB.Get(&exists, checkDislikeQuery, userID, markerID)
	return exists, err
}

func (s *MarkerInteractService) CheckUserFavorite(userID, markerID int) (bool, error) {
	var exists bool
	err := s.DB.Get(&exists, checkFavQuery, userID, markerID)
	return exists, err
}

// AddFavoriteHandler adds a new favorite marker for the user.
func (s *MarkerInteractService) AddFavorite(userID, markerID int) error {
	// First, count the existing favorites for the user
	var count int
	err := s.DB.QueryRowx(countFavQuery, userID).Scan(&count)
	if err != nil {
		return err
	}

	// Check if the user already has 10 favorites
	if count >= 10 {
		return errors.New("maximum number of favorites reached")
	}

	// If not, insert the new favorite
	_, err = s.DB.Exec(insertFavQuery, userID, markerID)
	if err != nil {
		// Convert error to string and check if it contains the MySQL error code for duplicate entry
		if strings.Contains(err.Error(), "1062") {
			return errors.New("you have already favorited this marker")
		}
		return fmt.Errorf("failed to add favorite: %w", err)
	}

	// Retrieve the UserID (owner) of the marker
	var ownerUserID int
	err = s.DB.QueryRowx(checkMarkerOwnerQuery, markerID).Scan(&ownerUserID)
	if err != nil {
		return fmt.Errorf("failed to retrieve marker owner: %w", err)
	}

	// if ownerUserID != userID {
	// 	userIDstr := strconv.Itoa(ownerUserID)
	// 	updateMsg := fmt.Sprintf("누군가 %d 마커에 좋아요를 남겼습니다!", markerID)
	// 	metadata := notification.NotificationLikeMetadata{
	// 		MarkerID: markerID,
	// 		UserId:   ownerUserID,
	// 		LikerId:  userID,
	// 	}

	// 	rawMetadata, _ := json.Marshal(metadata)
	// 	PostNotification(userIDstr, "Like", "sys", updateMsg, rawMetadata)
	// }

	// TODO: update when frontend updates
	// key := fmt.Sprintf("%d-%d", ownerUserID, markerID)
	// PublishLikeEvent(key)
	return nil
}

func (s *MarkerInteractService) RemoveFavorite(userID, markerID int) error {
	// Delete the specified favorite for the user
	_, err := s.DB.Exec(deleteFavQuery, userID, markerID)
	if err != nil {
		return fmt.Errorf("error removing favorite: %w", err)
	}
	return nil
}

func (s *MarkerInteractService) GetMarkersAfterID(lastMarkerID int) ([]dto.MarkersKakaoBot, error) {
	markers := []dto.MarkersKakaoBot{}
	err := s.DB.Select(&markers, getMarkersAfterIDQuery, lastMarkerID)
	return markers, err
}
