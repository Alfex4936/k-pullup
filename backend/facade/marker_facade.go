package facade

import (
	"fmt"
	"mime/multipart"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
)

// MarkerFacadeService provides a simplified interface to various marker-related services.
type MarkerFacadeService struct {
	InteractService *service.MarkerInteractService
	LocationService *service.MarkerLocationService
	ManageService   *service.MarkerManageService
	RankService     *service.MarkerRankService
	FacilityService *service.MarkerFacilityService
	RedisService    *service.RedisService
	ReportService   *service.ReportService

	UserService *service.UserService

	ChatUtil    *util.ChatUtil
	BadWordUtil *util.BadWordUtil
	MapUtil     *util.MapUtil
}

type MarkerFacadeParams struct {
	fx.In

	InteractService *service.MarkerInteractService
	LocationService *service.MarkerLocationService
	ManageService   *service.MarkerManageService
	RankService     *service.MarkerRankService
	FacilityService *service.MarkerFacilityService
	RedisService    *service.RedisService
	ReportService   *service.ReportService

	UserService *service.UserService

	ChatUtil    *util.ChatUtil
	BadWordUtil *util.BadWordUtil
	MapUtil     *util.MapUtil
}

func NewMarkerFacadeService(
	p MarkerFacadeParams,
) *MarkerFacadeService {
	return &MarkerFacadeService{
		InteractService: p.InteractService,
		LocationService: p.LocationService,
		ManageService:   p.ManageService,
		RankService:     p.RankService,
		FacilityService: p.FacilityService,
		RedisService:    p.RedisService,
		ReportService:   p.ReportService,
		UserService:     p.UserService,
		ChatUtil:        p.ChatUtil,
		BadWordUtil:     p.BadWordUtil,
		MapUtil:         p.MapUtil,
	}
}

// Get
func (mfs *MarkerFacadeService) GetMarker(markerID int) (*model.MarkerWithPhotos, error) {
	return mfs.ManageService.GetMarker(markerID)
}

func (mfs *MarkerFacadeService) GetAllMarkers() ([]dto.MarkerSimple, error) {
	return mfs.ManageService.GetAllMarkers()
}

func (mfs *MarkerFacadeService) GetAllNewMarkers(page, pageSize int) ([]dto.MarkerSimple, error) {
	return mfs.ManageService.GetAllNewMarkers(page, pageSize)
}

func (mfs *MarkerFacadeService) GetAllMarkersWithAddr() ([]dto.MarkerSimpleWithAddr, error) {
	return mfs.ManageService.GetAllMarkersWithAddr()
}

func (mfs *MarkerFacadeService) GetAllMarkersByUserWithPagination(userID, page, pageSize int) ([]dto.MarkerSimpleWithDescrption, int, error) {
	return mfs.ManageService.GetAllMarkersByUserWithPagination(userID, page, pageSize)
}

func (mfs *MarkerFacadeService) GetFacilitiesByMarkerID(markerID int) ([]model.Facility, error) {
	return mfs.FacilityService.GetFacilitiesByMarkerID(markerID)
}

func (mfs *MarkerFacadeService) CheckNearbyMarkersInDB() ([]dto.MarkerGroup, error) {
	return mfs.ManageService.CheckNearbyMarkersInDB()
}

func (mfs *MarkerFacadeService) GenerateRSS() (string, error) {
	return mfs.ManageService.GenerateRSS()
}

// Marker
func (mfs *MarkerFacadeService) CheckMarkerValidity(latitude, longitude float64, description string) *fiber.Error {
	return mfs.ManageService.CheckMarkerValidity(latitude, longitude, description)
}

func (mfs *MarkerFacadeService) CreateMarkerWithPhotos(markerDto *dto.MarkerRequest, userID int, form *multipart.Form) (*dto.MarkerResponse, error) {
	return mfs.ManageService.CreateMarkerWithPhotos(markerDto, userID, form)
}

func (mfs *MarkerFacadeService) UpdateMarkerDescriptionOnly(markerID int, description string) error {
	return mfs.ManageService.UpdateMarkerDescriptionOnly(markerID, description)
}

func (mfs *MarkerFacadeService) DeleteMarker(userID, markerID int, userRole string) error {
	return mfs.ManageService.DeleteMarker(userID, markerID, userRole)
}

func (mfs *MarkerFacadeService) UploadMarkerPhotoToS3(markerID int, files []*multipart.FileHeader) ([]string, error) {
	return mfs.ManageService.UploadMarkerPhotoToS3(markerID, files)
}

func (mfs *MarkerFacadeService) SetMarkerFacilities(markerID int, facilities []dto.FacilityQuantity) error {
	return mfs.FacilityService.SetMarkerFacilities(markerID, facilities)
}
func (mfs *MarkerFacadeService) UpdateMarkersAddresses() ([]dto.MarkerSimpleWithAddr, error) {
	return mfs.FacilityService.UpdateMarkersAddresses()
}

// RANK
func (mfs *MarkerFacadeService) BufferClickEvent(markerID int) {
	mfs.RankService.BufferClickEvent(markerID)
}

func (mfs *MarkerFacadeService) SaveUniqueVisitor(markerID string, c *fiber.Ctx) {
	if c != nil {
		mfs.RankService.SaveUniqueVisitor(markerID, mfs.ChatUtil.GetUserIP(c))
	}
}

func (mfs *MarkerFacadeService) RemoveMarkerClick(markerID int) error {
	return mfs.RankService.RemoveMarkerClick(markerID)
}

// CHAT
func (mfs *MarkerFacadeService) CheckBadWord(input string) (bool, error) {
	return mfs.BadWordUtil.CheckForBadWordsUsingTrie(input)
}

// LIKE/DISLIKE
func (mfs *MarkerFacadeService) AddFavorite(userID, markerID int) error {
	return mfs.InteractService.AddFavorite(userID, markerID)
}

func (mfs *MarkerFacadeService) RemoveFavorite(userID, markerID int) error {
	return mfs.InteractService.RemoveFavorite(userID, markerID)
}

func (mfs *MarkerFacadeService) CheckUserDislike(userID, markerID int) (bool, error) {
	return mfs.InteractService.CheckUserDislike(userID, markerID)
}

func (mfs *MarkerFacadeService) CheckUserFavorite(userID, markerID int) (bool, error) {
	return mfs.InteractService.CheckUserFavorite(userID, markerID)
}

func (mfs *MarkerFacadeService) LeaveDislike(userID, markerID int) error {
	return mfs.InteractService.LeaveDislike(userID, markerID)
}

func (mfs *MarkerFacadeService) UndoDislike(userID, markerID int) error {
	return mfs.InteractService.UndoDislike(userID, markerID)
}

// CACHING
func (mfs *MarkerFacadeService) GetMarkerCache() []byte {
	return mfs.ManageService.GetCache()
}

func (mfs *MarkerFacadeService) SetMarkerCache(mjson []byte) {
	mfs.ManageService.SetCache(mjson)
}

// Redis
func (mfs *MarkerFacadeService) GetRedisCache(key string, value interface{}) error {
	return mfs.RedisService.GetCacheEntry(key, value)
}

func (mfs *MarkerFacadeService) SetRedisCache(key string, value interface{}, expiration time.Duration) error {
	return mfs.RedisService.SetCacheEntry(key, value, expiration)
}

func (mfs *MarkerFacadeService) ResetRedisCache(key string) {
	mfs.RedisService.ResetCache(key)
}

func (mfs *MarkerFacadeService) ResetAllRedisCache(key string) {
	mfs.RedisService.ResetAllCache(key)
}

func (mfs *MarkerFacadeService) ResetFavCache(username string, userID int) error {
	userFavKey := fmt.Sprintf("%s:%d:%s", mfs.LocationService.Redis.RedisConfig.UserFavKey, userID, username)

	// Reset cache after adding to favorites
	return mfs.LocationService.Redis.ResetCache(userFavKey)
}

// User
func (mfs *MarkerFacadeService) GetUserFromContext(c *fiber.Ctx) (*dto.UserData, error) {
	return mfs.UserService.GetUserFromContext(c)
}
