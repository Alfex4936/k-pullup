package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	myconfig "github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

type S3Service struct {
	Config   *myconfig.S3Config
	Redis    *RedisService
	s3Client *s3.Client

	logger *zap.Logger
}

func NewS3Service(c *myconfig.S3Config, redis *RedisService, logger *zap.Logger) *S3Service {
	// Load the AWS credentials once
	awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(c.AwsRegion))
	if err != nil {
		return nil
	}

	// Create the S3 client once
	s3Client := s3.NewFromConfig(awsCfg)

	return &S3Service{
		Config:   c,
		Redis:    redis,
		logger:   logger,
		s3Client: s3Client,
	}
}

func (s *S3Service) UploadFileToS3(folder string, file *multipart.FileHeader, thumbnail bool) (string, string, error) {
	// Open the uploaded file
	fileData, err := file.Open()
	if err != nil {
		return "", "", err
	}
	defer fileData.Close()

	// Generate a UUID for a unique filename
	uuid, err := uuid.NewRandom()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate UUID: %w", err)
	}

	// Extract and lowercase the file extension
	ext := filepathExtLower(file.Filename)

	// Use the original file's extension but with a new UUID as the filename
	// Estimate the key length: folder + '/' + UUID + ext
	keyLen := len(folder) + 1 + 36 + len(ext)
	keyBytes := make([]byte, 0, keyLen)
	keyBytes = append(keyBytes, folder...)
	keyBytes = append(keyBytes, '/')
	keyBytes = appendUUID(keyBytes, uuid)
	keyBytes = append(keyBytes, ext...)

	// Convert keyBytes to string without allocation
	key := util.BytesToString(keyBytes)

	// Create a context with a timeout if necessary
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var thumbnailURL string

	// If thumbnail is requested and file is an image, generate the thumbnail
	if thumbnail && isImage(ext) {
		thumbnailURL, err = s.GenerateThumbnail(ctx, fileData, folder, uuid.String(), ext)
		if err != nil {
			s.logger.Error("failed to generate or upload thumbnail", zap.Error(err))
		}

		// Reset fileData to the beginning for uploading the original file
		_, err = fileData.Seek(0, io.SeekStart)
		if err != nil {
			return "", "", fmt.Errorf("failed to seek fileData: %w", err)
		}
	}

	// Upload the file to S3
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Config.S3BucketName,
		Key:    &key,
		Body:   fileData,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload file to S3: %w", err)
	}

	// Construct the file URL
	// Estimate the URL length: "https://" + bucket + ".s3.amazonaws.com/" + key
	urlLen := 8 + len(s.Config.S3BucketName) + 17 + len(key)
	urlBytes := make([]byte, 0, urlLen)
	urlBytes = append(urlBytes, "https://"...)
	urlBytes = append(urlBytes, s.Config.S3BucketName...)
	urlBytes = append(urlBytes, ".s3.amazonaws.com/"...)
	urlBytes = append(urlBytes, keyBytes...)

	// Convert urlBytes to string without allocation
	fileURL := util.BytesToString(urlBytes)

	return fileURL, thumbnailURL, nil
}

func (s *S3Service) UploadFileToS3WithContext(ctx context.Context, folder string, file *multipart.FileHeader, thumbnail bool) (string, string, error) {
	// Open the uploaded file
	fileData, err := file.Open()
	if err != nil {
		return "", "", err
	}
	defer fileData.Close()

	// Generate a UUID for a unique filename
	uuid, err := uuid.NewRandom()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate UUID: %w", err)
	}

	// Extract and lowercase the file extension
	ext := filepathExtLower(file.Filename)

	// Use the original file's extension but with a new UUID as the filename
	// Estimate the key length: folder + '/' + UUID + ext
	keyLen := len(folder) + 1 + 36 + len(ext)
	keyBytes := make([]byte, 0, keyLen)
	keyBytes = append(keyBytes, folder...)
	keyBytes = append(keyBytes, '/')
	keyBytes = appendUUID(keyBytes, uuid)
	keyBytes = append(keyBytes, ext...)

	// Convert keyBytes to string without allocation
	key := util.BytesToString(keyBytes)

	var thumbnailURL string

	// If thumbnail is requested and file is an image, generate the thumbnail
	if thumbnail && isImage(ext) {
		thumbnailURL, err = s.GenerateThumbnail(ctx, fileData, folder, uuid.String(), ext)
		if err != nil {
			s.logger.Error("failed to generate or upload thumbnail", zap.Error(err))
		}

		// Reset fileData to the beginning for uploading the original file
		_, err = fileData.Seek(0, io.SeekStart)
		if err != nil {
			return "", "", fmt.Errorf("failed to seek fileData: %w", err)
		}
	}

	// Upload the file to S3
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Config.S3BucketName,
		Key:    &key,
		Body:   fileData,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload file to S3: %w", err)
	}

	// Construct the file URL
	// Estimate the URL length: "https://" + bucket + ".s3.amazonaws.com/" + key
	urlLen := 8 + len(s.Config.S3BucketName) + 17 + len(key)
	urlBytes := make([]byte, 0, urlLen)
	urlBytes = append(urlBytes, "https://"...)
	urlBytes = append(urlBytes, s.Config.S3BucketName...)
	urlBytes = append(urlBytes, ".s3.amazonaws.com/"...)
	urlBytes = append(urlBytes, keyBytes...)

	// Convert urlBytes to string without allocation
	fileURL := util.BytesToString(urlBytes)

	return fileURL, thumbnailURL, nil
}

// UploadFileToS3WithImage uploads a file to S3 and returns the URLs along with the decoded image.
// This method reads the file only once, improving performance when the image is needed for further processing (e.g., blurhash).
func (s *S3Service) UploadFileToS3WithImage(ctx context.Context, folder string, file *multipart.FileHeader, thumbnail bool) (string, string, image.Image, error) {
	// Open the uploaded file
	fileData, err := file.Open()
	if err != nil {
		return "", "", nil, err
	}
	defer fileData.Close()

	// Read the entire file into memory once
	fileBytes, err := io.ReadAll(fileData)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Decode the image from the bytes
	img, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate a UUID for a unique filename
	uuid, err := uuid.NewRandom()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	// Extract and lowercase the file extension
	ext := filepathExtLower(file.Filename)

	// Use the original file's extension but with a new UUID as the filename
	// Estimate the key length: folder + '/' + UUID + ext
	keyLen := len(folder) + 1 + 36 + len(ext)
	keyBytes := make([]byte, 0, keyLen)
	keyBytes = append(keyBytes, folder...)
	keyBytes = append(keyBytes, '/')
	keyBytes = appendUUID(keyBytes, uuid)
	keyBytes = append(keyBytes, ext...)

	// Convert keyBytes to string without allocation
	key := util.BytesToString(keyBytes)

	var thumbnailURL string

	// If thumbnail is requested and file is an image, generate the thumbnail
	if thumbnail && isImage(ext) {
		// Use the already-decoded image to generate thumbnail, avoiding redundant decoding
		thumbnailURL, err = s.GenerateThumbnailFromImage(ctx, img, folder, uuid.String(), ext)
		if err != nil {
			s.logger.Error("failed to generate or upload thumbnail", zap.Error(err))
		}
	}

	// Upload the file to S3 using the bytes we've already read
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Config.S3BucketName,
		Key:    &key,
		Body:   bytes.NewReader(fileBytes),
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to upload file to S3: %w", err)
	}

	// Construct the file URL
	// Estimate the URL length: "https://" + bucket + ".s3.amazonaws.com/" + key
	urlLen := 8 + len(s.Config.S3BucketName) + 17 + len(key)
	urlBytes := make([]byte, 0, urlLen)
	urlBytes = append(urlBytes, "https://"...)
	urlBytes = append(urlBytes, s.Config.S3BucketName...)
	urlBytes = append(urlBytes, ".s3.amazonaws.com/"...)
	urlBytes = append(urlBytes, keyBytes...)

	// Convert urlBytes to string without allocation
	fileURL := util.BytesToString(urlBytes)

	return fileURL, thumbnailURL, img, nil
}

// DeleteDataFromS3 deletes a photo and its thumbnail from S3 given its URL.
func (s *S3Service) DeleteDataFromS3(dataURL string) error {
	var bucketName, key string

	// Attempt to parse the input as a URL
	parsedURL, err := url.Parse(dataURL)
	if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
		// It's a valid URL
		parts := strings.SplitN(parsedURL.Host, ".", 2)
		if len(parts) < 2 {
			return errors.New("invalid S3 URL format")
		}
		bucketName = parts[0]
		key = strings.TrimPrefix(parsedURL.Path, "/")
	} else {
		// It's not a valid URL, treat it as a key
		bucketName = s.Config.S3BucketName
		key = dataURL
	}

	if key == "" {
		return errors.New("invalid key")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Delete the object(s)
	keysToDelete := []string{key}

	// Check if the key does not contain '_thumb' and is an image
	ext := strings.ToLower(filepath.Ext(key))
	if !strings.Contains(key, "_thumb") && isImage(ext) {
		// Generate the thumbnail key
		thumbKey := generateThumbnailKey(key)
		keysToDelete = append(keysToDelete, thumbKey)
	}

	// Delete the objects
	deleteObjectsInput := &s3.DeleteObjectsInput{
		Bucket: &bucketName,
		Delete: &types.Delete{
			Objects: make([]types.ObjectIdentifier, len(keysToDelete)),
			Quiet:   aws.Bool(true),
		},
	}

	for i, k := range keysToDelete {
		deleteObjectsInput.Delete.Objects[i] = types.ObjectIdentifier{Key: &k}
	}

	_, err = s.s3Client.DeleteObjects(ctx, deleteObjectsInput)
	if err != nil {
		return fmt.Errorf("failed to delete object(s) from S3: %w", err)
	}

	return nil
}

func (s *S3Service) ListAllObjectsInS3() ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load the AWS credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(s.Config.AwsRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	input := &s3.ListObjectsV2Input{
		Bucket: &s.Config.S3BucketName,
	}

	var s3Objects []map[string]interface{}
	var sumKB int64
	paginator := s3.NewListObjectsV2Paginator(s3Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("error listing S3 objects: %w", err)
		}
		for _, item := range output.Contents {
			sizeKB := *item.Size / 1024 // Size in KB

			sumKB += sizeKB

			s3Objects = append(s3Objects, map[string]interface{}{
				"Key":  *item.Key,
				"Size": sizeKB,
			})
		}
	}

	s.logger.Info("💖 Total image size",
		zap.Int("number_of_images", len(s3Objects)),
		zap.Int64("total_size_kb", sumKB),
		zap.Int64("total_size_mb", sumKB/1024),
	)
	return s3Objects, nil
}

// FindUnreferencedS3Objects finds S3 objects that are not referenced in the database.
func (s *S3Service) FindUnreferencedS3Objects(dbURLs []string, s3Keys []string) []string {
	dbURLMap := make(map[string]struct{})
	for _, dbURL := range dbURLs {
		parsedURL, err := url.Parse(dbURL)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			key := strings.TrimPrefix(parsedURL.Path, "/")
			dbURLMap[key] = struct{}{}
		} else {
			// maybe just a key
			dbURLMap[dbURL] = struct{}{}
		}
	}

	var unreferenced []string
	for _, key := range s3Keys {
		if _, found := dbURLMap[key]; !found {
			unreferenced = append(unreferenced, key)
		}
	}

	return unreferenced
}

func (s *S3Service) MoveFileInS3(sourceKey string, destinationKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load the AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(s.Config.AwsRegion),
	)
	if err != nil {
		return fmt.Errorf("could not load AWS configuration: %w", err)
	}

	// Create an S3 client
	s3Client := s3.NewFromConfig(cfg)

	copySource := url.PathEscape(s.Config.S3BucketName + "/" + sourceKey)

	// Copy the object to the new location
	_, err = s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.Config.S3BucketName),
		CopySource: aws.String(copySource),
		Key:        aws.String(destinationKey),
	})
	if err != nil {
		return fmt.Errorf("failed to copy file in S3: %w", err)
	}

	// Delete the original object
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Config.S3BucketName),
		Key:    aws.String(sourceKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete original file in S3: %w", err)
	}

	return nil
}

// GenerateThumbnail generates a thumbnail for an image and uploads it to S3
func (s *S3Service) GenerateThumbnail(ctx context.Context, fileData multipart.File, folder, uuidStr, ext string) (string, error) {
	// Reset the fileData to the beginning
	_, err := fileData.Seek(0, io.SeekStart)
	if err != nil {
		return "", fmt.Errorf("failed to seek fileData: %w", err)
	}

	// Decode the image
	img, _, err := image.Decode(fileData)
	if err != nil {
		return "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate thumbnail
	thumbImg := imaging.Thumbnail(img, 300, 300, imaging.Lanczos)

	// Encode thumbnail to buffer
	var buf bytes.Buffer
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(&buf, thumbImg, nil)
	case ".png":
		err = png.Encode(&buf, thumbImg)
	case ".gif":
		err = gif.Encode(&buf, thumbImg, nil)
	case ".webp":
		err = webp.Encode(&buf, thumbImg, nil)
	default:
		return "", fmt.Errorf("unsupported image format: %s", ext)
	}
	if err != nil {
		return "", fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	// Generate thumbnail key (e.g., append "_thumb" before extension)
	thumbKey := fmt.Sprintf("%s/%s_thumb%s", folder, uuidStr, ext)

	// Upload thumbnail to S3
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Config.S3BucketName,
		Key:    &thumbKey,
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to S3: %w", err)
	}

	thumbURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Config.S3BucketName, thumbKey)

	return thumbURL, nil
}

// GenerateThumbnailFromImage generates a thumbnail from an already-decoded image.
// This method is more efficient when the image has already been decoded for other purposes.
func (s *S3Service) GenerateThumbnailFromImage(ctx context.Context, img image.Image, folder, uuidStr, ext string) (string, error) {
	// Generate thumbnail
	thumbImg := imaging.Thumbnail(img, 300, 300, imaging.Lanczos)

	// Encode thumbnail to buffer
	var buf bytes.Buffer
	switch ext {
	case ".jpg", ".jpeg":
		err := jpeg.Encode(&buf, thumbImg, nil)
		if err != nil {
			return "", fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	case ".png":
		err := png.Encode(&buf, thumbImg)
		if err != nil {
			return "", fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	case ".gif":
		err := gif.Encode(&buf, thumbImg, nil)
		if err != nil {
			return "", fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	case ".webp":
		err := webp.Encode(&buf, thumbImg, nil)
		if err != nil {
			return "", fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported image format: %s", ext)
	}

	// Generate thumbnail key (e.g., append "_thumb" before extension)
	thumbKey := fmt.Sprintf("%s/%s_thumb%s", folder, uuidStr, ext)

	// Upload thumbnail to S3
	_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Config.S3BucketName,
		Key:    &thumbKey,
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to S3: %w", err)
	}

	thumbURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Config.S3BucketName, thumbKey)

	return thumbURL, nil
}

// ObjectExists checks if an object exists in S3
func (s *S3Service) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Config.S3BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Helper function to determine if a file extension corresponds to an image
func isImage(ext string) bool {
	// Normalize the extension to lower case
	ext = strings.ToLower(ext)
	// List of supported image extensions
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

// appendUUID appends the UUID in standard string format to the dst byte slice.
func appendUUID(dst []byte, u uuid.UUID) []byte {
	for i, b := range u {
		// Insert dashes at positions 8, 13, 18, and 23.
		if i == 4 || i == 6 || i == 8 || i == 10 {
			dst = append(dst, '-')
		}
		dst = append(dst, util.HexDigits[b>>4], util.HexDigits[b&0x0f])
	}
	return dst
}

// toLowerASCII converts ASCII uppercase letters to lowercase in the byte slice.
// It modifies the slice in place and returns the modified slice.
func toLowerASCII(b []byte) []byte {
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return b
}

// filepathExtLower extracts the file extension and converts it to lowercase without allocations.
func filepathExtLower(filename string) string {
	ext := filepathExt(filename)
	if len(ext) == 0 {
		return ""
	}
	extBytes := []byte(ext)
	extBytes = toLowerASCII(extBytes)
	return util.BytesToString(extBytes)
}

// filepathExt is a simplified version of filepath.Ext.
// It returns the extension including the dot, or an empty string if none.
func filepathExt(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}

// Helper function to generate the thumbnail key from the original key
func generateThumbnailKey(originalKey string) string {
	ext := filepath.Ext(originalKey)
	baseName := strings.TrimSuffix(originalKey, ext)
	thumbKey := fmt.Sprintf("%s_thumb%s", baseName, ext)
	return thumbKey
}

// getContentType returns the MIME type based on file extension
func getContentType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
