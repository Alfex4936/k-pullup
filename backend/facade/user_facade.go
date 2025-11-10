package facade

import (
	"context"
	"fmt"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/gofiber/fiber/v2"
)

// UserFacadeService provides a simplified interface to various user-related services.
type UserFacadeService struct {
	UserService   *service.UserService
	RedisService  *service.RedisService
	ReportService *service.ReportService
	S3Service     *service.S3Service
}

func NewUserFacadeService(
	user *service.UserService,
	redis *service.RedisService,
	reporter *service.ReportService,
	s3 *service.S3Service,
) *UserFacadeService {
	return &UserFacadeService{
		UserService:  user,
		RedisService: redis,
		S3Service:    s3,
	}
}

func (mfs *UserFacadeService) GetUserFromContext(c *fiber.Ctx) (*dto.UserData, error) {
	return mfs.UserService.GetUserFromContext(c)
}

func (mfs *UserFacadeService) UpdateUserProfile(userID int, updateReq *dto.UpdateUserRequest) (*dto.UserResponse, error) {
	return mfs.UserService.UpdateUserProfile(userID, updateReq)
}

func (mfs *UserFacadeService) ResetUserFavCache(userID int) error {
	userProfileKey := fmt.Sprintf("%s:%d", mfs.RedisService.RedisConfig.UserFavKey, userID)
	return mfs.RedisService.ResetCache(userProfileKey)
}

func (mfs *UserFacadeService) GetUserProfileKey(userID int) string {
	userProfileKey := fmt.Sprintf("%s:%d", mfs.RedisService.RedisConfig.UserFavKey, userID)
	return userProfileKey
}
func (mfs *UserFacadeService) GetUserFavKey(userID int, username string) string {
	userFavKey := fmt.Sprintf("%s:%d:%s", mfs.RedisService.RedisConfig.UserFavKey, userID, username)
	return userFavKey
}

func (mfs *UserFacadeService) GetUserCache(key string, value interface{}) error {
	return mfs.RedisService.GetCacheEntry(key, value)
}

func (mfs *UserFacadeService) GetUserById(userID int) (*dto.UserResponse, error) {
	return mfs.UserService.GetUserById(userID)
}

func (mfs *UserFacadeService) GetUserStatistics(userID int) (int, int, error) {
	return mfs.UserService.GetUserStatistics(userID)
}

func (mfs *UserFacadeService) SetRedisCache(key string, value interface{}, expiration time.Duration) error {
	return mfs.RedisService.SetCacheEntry(key, value, expiration)
}

func (mfs *UserFacadeService) GetAllFavorites(userID int) ([]dto.MarkerSimpleWithDescription, error) {
	return mfs.UserService.GetAllFavorites(userID)
}

func (mfs *UserFacadeService) GetAllReportsByUser(userID int) ([]dto.MarkerReportResponse, error) {
	return mfs.UserService.GetAllReportsByUser(userID)
}

func (mfs *UserFacadeService) GetAllReportsForMyMarkersByUser(userID int) (dto.GroupedReportsResponse, error) {
	return mfs.UserService.GetAllReportsForMyMarkersByUser(userID)
}

func (mfs *UserFacadeService) DeleteDataFromS3(dataURL string) error {
	return mfs.S3Service.DeleteDataFromS3(dataURL)
}

func (mfs *UserFacadeService) DeleteUserWithRelatedData(ctx context.Context, userID int) error {
	return mfs.UserService.DeleteUserWithRelatedData(ctx, userID)
}
