package services

import (
	"chulbong-kr/database"
	"chulbong-kr/dto"
	"chulbong-kr/models"
	"fmt"
	"time"
)

type Comment = models.Comment

// CreateComment inserts a new comment into the database
func CreateComment(markerID, userID int, commentText string) (*Comment, error) {
	// First, check if the marker exists
	var exists bool
	markerCheckQuery := `SELECT EXISTS(SELECT 1 FROM Markers WHERE MarkerID = ?)`
	err := database.DB.Get(&exists, markerCheckQuery, markerID)
	if err != nil {
		return nil, fmt.Errorf("error checking if marker exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("marker with ID %d does not exist", markerID)
	}

	// Create the comment instance
	comment := Comment{
		MarkerID:    markerID,
		UserID:      userID,
		CommentText: commentText,
		PostedAt:    time.Now(),
		UpdatedAt:   time.Now(),
		DeletedAt:   nil,
	}

	// Insert into database
	query := `INSERT INTO Comments (MarkerID, UserID, CommentText, PostedAt, UpdatedAt)
              VALUES (?, ?, ?, ?, ?)`
	res, err := database.DB.Exec(query, comment.MarkerID, comment.UserID, comment.CommentText, comment.PostedAt, comment.UpdatedAt)
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

// UpdateComment updates an existing comment made by a user.
func UpdateComment(commentID int, userID int, newCommentText string) error {
	// SQL query to update the comment text for a given commentID and userID
	query := `UPDATE Comments SET CommentText = ?, UpdatedAt = NOW() WHERE CommentID = ? AND UserID = ? AND DeletedAt IS NULL`
	res, err := database.DB.Exec(query, newCommentText, commentID, userID)
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

func RemoveComment(commentID, userID int) error {
	// Soft delete the comment by setting the DeletedAt timestamp
	query := `UPDATE Comments SET DeletedAt = NOW() WHERE CommentID = ? AND UserID = ? AND DeletedAt IS NULL`
	res, err := database.DB.Exec(query, commentID, userID)
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
func LoadCommentsForMarker(markerID, pageSize, offset int) ([]dto.CommentWithUsername, int, error) {
	comments := make([]dto.CommentWithUsername, 0)

	query := `
		SELECT C.CommentID, C.MarkerID, C.UserID, C.CommentText, C.PostedAt, C.UpdatedAt, U.Username
		FROM Comments C
		LEFT JOIN Users U ON C.UserID = U.UserID
		WHERE C.MarkerID = ? AND C.DeletedAt IS NULL
        ORDER BY C.PostedAt DESC
		LIMIT ? OFFSET ?`

	err := database.DB.Select(&comments, query, markerID, pageSize, offset) // SELECT has to flatten the struct
	if err != nil {
		return nil, 0, fmt.Errorf("error loading comments for marker %d: %w", markerID, err)
	}

	countQuery := `
SELECT COUNT(*)
FROM Comments C
WHERE C.MarkerID = ? AND C.DeletedAt IS NULL`

	var total int
	err = database.DB.Get(&total, countQuery, markerID)
	if err != nil {
		return nil, 0, fmt.Errorf("error getting total markers count: %w", err)
	}

	return comments, total, nil
}
