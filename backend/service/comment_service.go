package service

import (
	"fmt"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/jmoiron/sqlx"
)

const (
	markerCheckQuery   = "SELECT EXISTS(SELECT 1 FROM Markers WHERE MarkerID = ?)"
	commentCountQuery  = "SELECT COUNT(*) FROM Comments WHERE MarkerID = ? AND UserID = ? AND DeletedAt IS NULL"
	insertCommentQuery = "INSERT INTO Comments (MarkerID, UserID, CommentText, PostedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?)"
	updateCommentQuery = "UPDATE Comments SET CommentText = ?, UpdatedAt = NOW() WHERE CommentID = ? AND UserID = ? AND DeletedAt IS NULL"
	removeCommentQuery = `
UPDATE Comments
SET DeletedAt = NOW()
WHERE CommentID = ?
	AND DeletedAt IS NULL
	AND (UserID = ? OR EXISTS (
		SELECT 1 FROM Users 
		WHERE Users.UserID = ? AND Users.Role = 'admin'
	))`
	loadAllCommentsQuery = `
SELECT C.CommentID, C.MarkerID, C.UserID, C.CommentText, C.PostedAt, C.UpdatedAt, U.Username
FROM Comments C
LEFT JOIN Users U ON C.UserID = U.UserID
WHERE C.MarkerID = ? AND C.DeletedAt IS NULL
ORDER BY 
    (U.Username = 'k-pullup') DESC, 
    C.PostedAt DESC
LIMIT ? OFFSET ?`

	countCommentQuery = `
SELECT COUNT(*)
FROM Comments C
WHERE C.MarkerID = ? AND C.DeletedAt IS NULL`
)

type MarkerCommentService struct {
	DB           *sqlx.DB
	RedisService *RedisService
}

func NewMarkerCommentService(db *sqlx.DB, redisService *RedisService) *MarkerCommentService {
	return &MarkerCommentService{
		DB:           db,
		RedisService: redisService,
	}
}

type Comment = model.Comment

// GetUserCommentCreationStatus returns the current and remaining comment creation count for a user today
func (s *MarkerCommentService) GetUserCommentCreationStatus(userID int) (current int64, remaining int, err error) {
	today := time.Now().Format("2006-01-02")

	current, err = s.RedisService.GetCommentCreateCount(userID, today)
	if err != nil {
		return 0, 0, err
	}

	remaining, err = s.RedisService.GetRemainingCommentCreates(userID, today)
	if err != nil {
		return current, 0, err
	}

	return current, remaining, nil
}

// CreateComment inserts a new comment into the database
func (s *MarkerCommentService) CreateComment(markerID, userID int, userName, commentText string) (*dto.CommentWithUsername, error) {
	// First, check if the marker exists
	var exists bool
	err := s.DB.Get(&exists, markerCheckQuery, markerID)
	if err != nil {
		return nil, fmt.Errorf("error checking if marker exists: %w", err)
	}
	if !exists {
		return nil, ErrMarkerNotFound
	}

	// Check daily comment creation limit (15 per day per user)
	today := time.Now().Format("2006-01-02") // YYYY-MM-DD format
	if userID != 1 {                         // k-pullup (admin) is exempt from daily limit
		currentCount, err := s.RedisService.GetCommentCreateCount(userID, today)
		if err != nil {
			// Log error but continue with creation if Redis fails (fail open)
			// Could add logging here if logger is available
		} else if currentCount >= 15 {
			return nil, ErrDailyCommentLimitReached
		}
	}

	// Check if the user has already commented 3 times on this marker
	var commentCount int
	err = s.DB.Get(&commentCount, commentCountQuery, markerID, userID)
	if err != nil {
		return nil, fmt.Errorf("error checking comment count: %w", err)
	}
	if userID != 1 && commentCount >= 3 { // k-pullup can comment more than 3
		return nil, ErrMaxCommentsReached
	}

	// Create the comment instance
	comment := dto.CommentWithUsername{
		MarkerID:    markerID,
		UserID:      userID,
		CommentText: commentText,
		PostedAt:    time.Now(),
		UpdatedAt:   time.Now(),
		Username:    userName,
	}

	// Insert into database
	res, err := s.DB.Exec(insertCommentQuery, comment.MarkerID, comment.UserID, comment.CommentText, comment.PostedAt, comment.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Fetch the last inserted ID
	lastID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	comment.CommentID = int(lastID)

	// Increment the daily comment creation count for the user (only if not admin)
	if userID != 1 {
		go func() {
			_, err := s.RedisService.IncrementCommentCreateCount(userID, today)
			if err != nil {
				// Log error but don't fail the operation
				// Could add logging here if logger is available
			}
		}()

		// Send new comment to slack only if not admin
		go util.SendSlackNewComment(markerID, userID, userName, commentText)
	}

	return &comment, nil
}

// UpdateComment updates an existing comment made by a user.
func (s *MarkerCommentService) UpdateComment(commentID int, userID int, newCommentText string) error {
	// SQL query to update the comment text for a given commentID and userID
	res, err := s.DB.Exec(updateCommentQuery, newCommentText, commentID, userID)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}

	// Check if the comment was actually updated
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking updated comment: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("comment not found or not owned by user")
	}

	return nil
}

func (s *MarkerCommentService) RemoveComment(commentID, userID int) error {
	// Soft delete the comment by setting the DeletedAt timestamp
	res, err := s.DB.Exec(removeCommentQuery, commentID, userID, userID)
	if err != nil {
		return err
	}

	// Check if any row was updated
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("comment not found or already deleted")
	}

	return nil
}

// LoadCommentsForMarker retrieves all active comments for a specific marker
func (s *MarkerCommentService) LoadCommentsForMarker(markerID, pageSize, offset int) ([]dto.CommentWithUsername, int, error) {
	comments := make([]dto.CommentWithUsername, 0)

	err := s.DB.Select(&comments, loadAllCommentsQuery, markerID, pageSize, offset) // SELECT has to flatten the struct
	if err != nil {
		return nil, 0, fmt.Errorf("error loading comments for marker %d: %w", markerID, err)
	}

	var total int
	err = s.DB.Get(&total, countCommentQuery, markerID)
	if err != nil {
		return nil, 0, fmt.Errorf("error getting total markers count: %w", err)
	}

	return comments, total, nil
}

func (s *MarkerCommentService) CreateCommentTx(tx *sqlx.Tx, markerID, userID int, username, commentText string) (*dto.CommentWithUsername, error) {
	// Check if the marker exists
	var exists bool
	err := tx.Get(&exists, markerCheckQuery, markerID)
	if err != nil {
		return nil, fmt.Errorf("error checking if marker exists: %w", err)
	}
	if !exists {
		return nil, ErrMarkerNotFound
	}

	// Create the comment instance
	comment := dto.CommentWithUsername{
		MarkerID:    markerID,
		UserID:      userID,
		CommentText: commentText,
		PostedAt:    time.Now(),
		UpdatedAt:   time.Now(),
		Username:    username,
	}

	// Insert into database
	res, err := tx.Exec(insertCommentQuery, comment.MarkerID, comment.UserID, comment.CommentText, comment.PostedAt, comment.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Fetch the last inserted ID
	lastID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	comment.CommentID = int(lastID)

	return &comment, nil
}
