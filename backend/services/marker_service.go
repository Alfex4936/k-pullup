package services

import (
	"errors"
	"fmt"
	"math"
	"mime/multipart"

	"chulbong-kr/database"
	"chulbong-kr/dto"
	"chulbong-kr/models"
)

// CreateMarker creates a new marker in the database after checking for nearby markers
func CreateMarker(markerDto *dto.MarkerRequest, userId int) (*models.Marker, error) {
	// Start a transaction
	tx, err := database.DB.Beginx()
	if err != nil {
		return nil, err
	}
	// Ensure the transaction is rolled back if any step fails
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // re-throw panic after Rollback
		} else if err != nil {
			tx.Rollback() // err is non-nil; don't change it
		} else {
			err = tx.Commit() // if Commit returns error update err with commit err
		}
	}()

	// First, check if there is a nearby marker
	nearby, err := IsMarkerNearby(markerDto.Latitude, markerDto.Longitude)
	if err != nil {
		return nil, err // Return any error encountered
	}
	if nearby {
		return nil, errors.New("a marker is already nearby")
	}

	// Insert the new marker within the transaction
	const insertQuery = `INSERT INTO Markers (UserID, Latitude, Longitude, Description, CreatedAt, UpdatedAt) 
                         VALUES (?, ?, ?, ?, NOW(), NOW())`
	res, err := tx.Exec(insertQuery, userId, markerDto.Latitude, markerDto.Longitude, markerDto.Description)
	if err != nil {
		return nil, fmt.Errorf("inserting marker: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert ID: %w", err)
	}

	// Create a marker model instance to hold the full marker information
	marker := &models.Marker{
		MarkerID:    int(id),
		UserID:      userId,
		Latitude:    markerDto.Latitude,
		Longitude:   markerDto.Longitude,
		Description: markerDto.Description,
	}

	// Fetch the newly created marker to populate all fields, including CreatedAt and UpdatedAt
	// const selectQuery = `SELECT CreatedAt, UpdatedAt FROM Markers WHERE MarkerID = ?`
	// err = database.DB.QueryRow(selectQuery, marker.MarkerID).Scan(&marker.CreatedAt, &marker.UpdatedAt)
	// if err != nil {
	// 	return nil, fmt.Errorf("fetching created marker: %w", err)
	// }

	return marker, nil
}

func CreateMarkerWithPhotos(markerDto *dto.MarkerRequest, userID int, form *multipart.Form) (*dto.MarkerResponse, error) {
	// Begin a transaction for database operations
	tx, err := database.DB.Beginx()
	if err != nil {
		return nil, err
	}

	// Insert the marker into the database
	res, err := tx.Exec(
		"INSERT INTO Markers (UserID, Latitude, Longitude, Description, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, NOW(), NOW())",
		userID, markerDto.Latitude, markerDto.Longitude, markerDto.Description,
	)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	markerID, _ := res.LastInsertId()

	// After successfully creating the marker, process and upload the files
	var photoURLs []string

	// Process file uploads from the multipart form
	files := form.File["photos"] // Assuming "photos" is the field name for files
	for _, file := range files {
		fileURL, err := UploadFileToS3(file)
		if err != nil {
			fmt.Printf("Failed to upload file to S3: %v\n", err)
			continue // Skip this file and continue with the next
		}

		photoURLs = append(photoURLs, fileURL)

		// Associate each photo with the marker in the database
		if _, err := tx.Exec("INSERT INTO Photos (MarkerID, PhotoURL, UploadedAt) VALUES (?, ?, NOW())", markerID, fileURL); err != nil {
			tx.Rollback()

			// Attempt to delete the uploaded file from S3
			if delErr := DeleteDataFromS3(fileURL); delErr != nil {
				fmt.Printf("Also failed to delete the file from S3: %v\n", delErr)
			}
			return nil, err
		}
	}

	// Commit the transaction after all operations succeed
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	// Construct and return the response
	return &dto.MarkerResponse{
		MarkerID:    int(markerID),
		Latitude:    markerDto.Latitude,
		Longitude:   markerDto.Longitude,
		Description: markerDto.Description,
		PhotoURLs:   photoURLs,
	}, nil
}

func GetAllMarkers() ([]models.MarkerWithPhotos, error) {
	// a query that joins Markers with Users to select the username as well
	const markerQuery = `
    SELECT Markers.*, Users.Username, COUNT(MarkerDislikes.DislikeID) AS DislikeCount
    FROM Markers
    JOIN Users ON Markers.UserID = Users.UserID
    LEFT JOIN MarkerDislikes ON Markers.MarkerID = MarkerDislikes.MarkerID
    GROUP BY Markers.MarkerID, Users.Username`

	var markersWithUsernames []struct {
		models.Marker
		Username     string `db:"Username"`
		DislikeCount int    `db:"DislikeCount"`
	}
	err := database.DB.Select(&markersWithUsernames, markerQuery)
	if err != nil {
		return nil, err
	}

	// Fetch all photos at once
	const photoQuery = `SELECT * FROM Photos`
	var allPhotos []models.Photo
	err = database.DB.Select(&allPhotos, photoQuery)
	if err != nil {
		return nil, err
	}

	// Map photos to their markers
	photoMap := make(map[int][]models.Photo) // markerID to photos
	for _, photo := range allPhotos {
		photoMap[photo.MarkerID] = append(photoMap[photo.MarkerID], photo)
	}

	// Assemble the final structure
	var markersWithPhotos []models.MarkerWithPhotos
	for _, marker := range markersWithUsernames {
		markersWithPhotos = append(markersWithPhotos, models.MarkerWithPhotos{
			Marker:       marker.Marker,
			Photos:       photoMap[marker.MarkerID],
			Username:     marker.Username,
			DislikeCount: marker.DislikeCount,
		})
	}

	return markersWithPhotos, nil
}

// GetMarker retrieves a single marker and its associated photo by the marker's ID
func GetMarker(markerID int) (*models.MarkerWithPhoto, error) {
	const query = `
	SELECT m.*, p.PhotoID, p.PhotoURL, p.UploadedAt 
	FROM Markers m
	LEFT JOIN Photos p ON m.MarkerID = p.MarkerID
	WHERE m.MarkerID = ?`

	var markerWithPhoto models.MarkerWithPhoto
	err := database.DB.Get(&markerWithPhoto, query, markerID)
	if err != nil {
		return nil, fmt.Errorf("fetching marker with photo: %w", err)
	}

	return &markerWithPhoto, nil
}

// UpdateMarker updates an existing marker's latitude, longitude, and description
func UpdateMarker(marker *models.Marker) error {
	const query = `UPDATE Markers SET Latitude = ?, Longitude = ?, Description = ?, UpdatedAt = NOW() 
                   WHERE MarkerID = ?`
	_, err := database.DB.Exec(query, marker.Latitude, marker.Longitude, marker.Description, marker.MarkerID)
	return err
}

// LeaveDislike user's dislike for a marker
func LeaveDislike(userID int, markerID int) error {
	_, err := database.DB.Exec(
		"INSERT INTO MarkerDislikes (MarkerID, UserID) VALUES (?, ?) ON DUPLICATE KEY UPDATE DislikedAt=VALUES(DislikedAt)",
		markerID, userID,
	)
	if err != nil {
		return fmt.Errorf("inserting dislike: %w", err)
	}
	return nil
}

// UndoDislike nudo user's dislike for a marker
func UndoDislike(userID int, markerID int) error {
	result, err := database.DB.Exec(
		"DELETE FROM MarkerDislikes WHERE UserID = ? AND MarkerID = ?",
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
		return fmt.Errorf("no dislike found to undo")
	}

	return nil
}

// This service function checks if the given user has disliked the specified marker.
func CheckUserDislike(userID, markerID int) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM MarkerDislikes WHERE UserID = ? AND MarkerID = ?)"
	err := database.DB.Get(&exists, query, userID, markerID)
	return exists, err
}

// DeleteMarker deletes a marker and its associated photos from the database and S3.
func DeleteMarker(userID, markerID int) error {
	// Start a transaction
	tx, err := database.DB.Beginx()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}

	// Check if the marker belongs to the user
	var ownerID int
	const checkOwnerQuery = `SELECT UserID FROM Markers WHERE MarkerID = ?`
	err = tx.Get(&ownerID, checkOwnerQuery, markerID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("checking marker ownership: %w", err)
	}
	if ownerID != userID {
		tx.Rollback()
		return fmt.Errorf("user %d is not authorized to delete marker %d", userID, markerID)
	}

	// fetch photo URLs associated with the marker
	var photoURLs []string
	const selectPhotosQuery = `SELECT PhotoURL FROM Photos WHERE MarkerID = ?`
	err = tx.Select(&photoURLs, selectPhotosQuery, markerID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("fetching photos: %w", err)
	}

	// Delete associated photos from the database
	const deletePhotosQuery = `DELETE FROM Photos WHERE MarkerID = ?`
	if _, err := tx.Exec(deletePhotosQuery, markerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting photos: %w", err)
	}

	// Now delete the marker
	const deleteMarkerQuery = `DELETE FROM Markers WHERE MarkerID = ?`
	if _, err := tx.Exec(deleteMarkerQuery, markerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting marker: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	// After successfully deleting from the database, delete photos from S3
	for _, photoURL := range photoURLs {
		if err := DeleteDataFromS3(photoURL); err != nil {
			return fmt.Errorf("deleting photo from S3: %w", err)
		}
	}

	return nil
}

// meters_per_degree = 40075000 / 360 / 1000
// IsMarkerNearby checks if there's a marker within 5 meters of the given latitude and longitude
func IsMarkerNearby(lat, long float64) (bool, error) {
	const query = `SELECT Latitude, Longitude FROM Markers`
	var markers []models.Marker
	err := database.DB.Select(&markers, query)
	if err != nil {
		return false, err
	}
	for _, m := range markers {
		if math.Abs(distance(lat, long, m.Latitude, m.Longitude)-5) < 1 { // allow 1 meter error
			return true, nil
		}
	}
	return false, nil
}

// distance calculates the distance between two geographic coordinates in meters
func distance(lat1, long1, lat2, long2 float64) float64 {
	var deltaLat = (lat2 - lat1) * (math.Pi / 180)
	var deltaLong = (long2 - long1) * (math.Pi / 180)
	var a = math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1*(math.Pi/180))*math.Cos(lat2*(math.Pi/180))*
			math.Sin(deltaLong/2)*math.Sin(deltaLong/2)
	var c = 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return 6371000 * c // Earth radius in meters
}
