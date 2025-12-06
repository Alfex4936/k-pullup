package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	sonic "github.com/bytedance/sonic"
	"github.com/goccy/go-json"
	"github.com/jmoiron/sqlx"

	"github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/dto/kakao"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/util"
)

const (
	// cost: 0.70, access_type: ref
	getFacilitiesQuery = "SELECT FacilityID, MarkerID, Quantity FROM MarkerFacilities WHERE MarkerID = ?"

	deleteFacilitiesQuery = "DELETE FROM MarkerFacilities WHERE MarkerID = ?"
	insertFacilitiesQuery = "INSERT INTO MarkerFacilities (FacilityID, MarkerID, Quantity) VALUES (?, ?, ?)"

	// cost: ~530.65, access_type: all
	getAllMarkersQuery = "SELECT MarkerID, ST_X(Location) AS Latitude, ST_Y(Location) AS Longitude FROM Markers"

	updateAddressQuery = "UPDATE Markers SET Address = ? WHERE MarkerID = ?"

	// userAgent
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

type MarkerFacilityService struct {
	Config      *config.AppConfig
	KakaoConfig *config.KakaoConfig
	DB          *sqlx.DB
	HTTPClient  *http.Client

	CacheService *MarkerCacheService
}

func NewMarkerFacilityService(
	config *config.AppConfig,
	kakaoConfig *config.KakaoConfig,
	db *sqlx.DB,
	httpClient *http.Client,
	c *MarkerCacheService) *MarkerFacilityService {
	return &MarkerFacilityService{
		Config:       config,
		KakaoConfig:  kakaoConfig,
		DB:           db,
		HTTPClient:   httpClient,
		CacheService: c,
	}
}

// GetFacilitiesByMarkerID retrieves facilities for a given marker ID.
func (s *MarkerFacilityService) GetFacilitiesByMarkerID(markerID int) ([]model.Facility, error) {
	facilities := make([]model.Facility, 0)
	err := s.DB.Select(&facilities, getFacilitiesQuery, markerID)
	if err != nil {
		return nil, err
	}
	return facilities, nil
}

func (s *MarkerFacilityService) SetMarkerFacilities(markerID int, facilities []dto.FacilityQuantity) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Remove existing facilities for the marker
	if _, err := tx.Exec(deleteFacilitiesQuery, markerID); err != nil {
		return err
	}

	// Insert new facilities with quantities for the marker
	for _, fq := range facilities {
		if _, err := tx.Exec(insertFacilitiesQuery, fq.FacilityID, markerID, fq.Quantity); err != nil {
			return err
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return err
	}

	s.CacheService.InvalidateFacilities(markerID)

	return nil
}

// UpdateMarkersAddresses fetches all markers, updates their addresses using an external API, and returns the updated list.
func (s *MarkerFacilityService) UpdateMarkersAddresses() ([]dto.MarkerSimpleWithAddr, error) {
	var markers []dto.MarkerSimpleWithAddr
	err := s.DB.Select(&markers, getAllMarkersQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	for i := range markers {
		address, err := s.FetchAddressFromAPI(markers[i].Latitude, markers[i].Longitude)
		if err != nil || address == "" {
			// If there's an error fetching the address or the address is not found, skip updating this marker.
			continue
		}

		markers[i].Address = address
		// Update the marker's address in the database.
		if err := s.UpdateMarkerAddress(markers[i].MarkerID, address); err != nil {
			// Log or handle the error based on your application's error handling policy
			fmt.Printf("Failed to update address for marker %d: %v\n", markers[i].MarkerID, err)
		}
	}

	return markers, nil
}

// UpdateMarkerAddress updates the address of a marker by its ID.
func (s *MarkerFacilityService) UpdateMarkerAddress(markerID int, address string) error {
	_, err := s.DB.Exec(updateAddressQuery, address, markerID)
	return err
}

// FetchAddressFromAPI queries the external API to get the address for a given latitude and longitude.
func (s *MarkerFacilityService) FetchAddressFromAPI(latitude, longitude float64) (string, error) {
	// Build URL manually
	var urlBuilder strings.Builder
	urlBuilder.Grow(len(s.KakaoConfig.KakaoCoord2Addr) + 50)
	urlBuilder.WriteString(s.KakaoConfig.KakaoCoord2Addr)
	urlBuilder.WriteString("?x=")
	urlBuilder.WriteString(strconv.FormatFloat(longitude, 'f', 6, 64))
	urlBuilder.WriteString("&y=")
	urlBuilder.WriteString(strconv.FormatFloat(latitude, 'f', 6, 64))
	reqURL := urlBuilder.String()

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "-1", err
	}
	req.Header.Add("Authorization", "KakaoAK "+s.KakaoConfig.KakaoAK)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "-1", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "-1", errors.New("unexpected status code")
	}

	var apiResp kakao.KakaoResponse
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "-1", err
	}

	if len(apiResp.Documents) == 0 {
		// No address found
		return "", nil
	}

	doc := apiResp.Documents[0]
	if doc.Address != nil && doc.Address.AddressName != "" {
		return doc.Address.AddressName, nil
	}
	if doc.RoadAddress != nil && doc.RoadAddress.AddressName != "" {
		return doc.RoadAddress.AddressName, nil
	}

	return "", nil
}

// FetchAddressFromMap queries the external API to get the address for a given latitude and longitude. (KakaoMap)
func (s *MarkerFacilityService) FetchAddressFromMap(latitude, longitude float64) (string, error) {
	wcongnamul := util.ConvertWGS84ToWCONGNAMUL(latitude, longitude)

	reqURL := fmt.Sprintf("%s&x=%f&y=%f", s.KakaoConfig.KakaoAddressInfo, wcongnamul.X, wcongnamul.Y)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp kakao.KakaoMarkerData
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	// Decide which address to return based on the presence of Old and New
	if apiResp.New != nil && *apiResp.New.Name != "" {
		address := *apiResp.New.Name
		if apiResp.New.Building != nil && *apiResp.New.Building != "" {
			address += ", " + *apiResp.New.Building
		}
		return address, nil
	} else if apiResp.Old != nil && *apiResp.Old.Name != "" {
		address := *apiResp.Old.Name
		if apiResp.Old.Building != nil && *apiResp.Old.Building != "" {
			address += ", " + *apiResp.Old.Building
		}
		return address, nil
	}

	return "", errors.New("no valid address found")
}

// FetchXYFromAPI queries the external API to get the latitude and longitude for a given address.
func (s *MarkerFacilityService) FetchXYFromAPI(address string) (float64, float64, error) {
	reqURL := fmt.Sprintf("%s?query=%s", s.KakaoConfig.KakaoGeoCode, address)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return 0.0, 0.0, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("Authorization", "KakaoAK "+s.KakaoConfig.KakaoAK)
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return 0.0, 0.0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0.0, 0.0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp kakao.KakaoResponse
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return 0.0, 0.0, fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(apiResp.Documents) == 0 {
		// log.Print("No address found for the given coordinates")
		return 0.0, 0.0, nil // Returning nil error to indicate absence of data rather than a failure
	}

	doc := apiResp.Documents[0]
	if doc.X != nil && doc.Y != nil {

		latitude, err := strconv.ParseFloat(*doc.X, 64)
		if err != nil {
			return 0, 0, errors.New("invalid latitude")
		}

		longitude, err := strconv.ParseFloat(*doc.Y, 64)
		if err != nil {
			return 0, 0, errors.New("invalid longitude")
		}

		return latitude, longitude, nil
	}

	return 0.0, 0.0, nil // Address data is empty but no error occurred
}

// FetchRegionFromAPI queries the external API to get the address for a given latitude and longitude.
func (s *MarkerFacilityService) FetchRegionFromAPI(latitude, longitude float64) (string, error) {
	reqURL := fmt.Sprintf("%s?x=%f&y=%f", s.KakaoConfig.KakaoCoord2Region, longitude, latitude)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "-1", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("Authorization", "KakaoAK "+s.KakaoConfig.KakaoAK)
	req.Header.Add("User-Agent", userAgent)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "-1", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "-1", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp kakao.KakaoRegionResponse
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "-1", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(apiResp.Documents) == 0 {
		// log.Print("No address found for the given coordinates")
		return "", nil // Returning nil error to indicate absence of data rather than a failure
	}

	doc := apiResp.Documents[0]
	switch doc.AddressName {
	case "북한", "일본":
		return "-2", nil
	default:
		return doc.AddressName, nil
	}
}

// FetchEnglishAddress fetches the English address for a given Korean address from the website.
func (s *MarkerFacilityService) FetchEnglishAddress(koreanAddress string) (string, error) {
	// URL encode the Korean address
	encodedAddress := strings.ReplaceAll(koreanAddress, " ", "+")
	reqURL := fmt.Sprintf("https://www.jusoen.com/addreng.asp?p1=%s", encodedAddress)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("User-Agent", userAgent)
	// Create a new HTTP request

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	// Use regex to find the English address in the response body
	re := regexp.MustCompile(`<strong>([^<]+)</strong>`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return "", fmt.Errorf("could not find English address in response")
	}

	// Remove commas from the address
	englishAddress := strings.ReplaceAll(matches[1], ",", "")

	return englishAddress, nil
}

// FetchWeatherFromAddress
func (s *MarkerFacilityService) FetchWeatherFromAddress(latitude, longitude float64) (*kakao.WeatherRequest, error) {
	wcongnamul := util.ConvertWGS84ToWCONGNAMUL(latitude, longitude)

	reqURL := fmt.Sprintf("%s&x=%f&y=%f", s.KakaoConfig.KakaoWeather, wcongnamul.X, wcongnamul.Y)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("Referer", reqURL)
	req.Header.Add("User-Agent", userAgent)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp kakao.WeatherResponse

	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if apiResp.Codes.ResultCode != "OK" {
		// log.Print("No weather found for this address")
		return nil, errors.New("no weather found for this address")
	}

	icon := fmt.Sprintf("https://t1.daumcdn.net/localimg/localimages/07/2018/pc/weather/ico_weather%s.png", apiResp.WeatherInfos.Current.IconId)
	weatherRequest := kakao.WeatherRequest{
		Temperature: apiResp.WeatherInfos.Current.Temperature,
		Desc:        apiResp.WeatherInfos.Current.Desc,
		IconImage:   icon,
		Humidity:    apiResp.WeatherInfos.Current.Humidity,
		Rainfall:    apiResp.WeatherInfos.Current.Rainfall,
		Snowfall:    apiResp.WeatherInfos.Current.Snowfall,
	}
	return &weatherRequest, nil
}

// FetchRegionWaterInfo checks if latitude/longitude is in the water possibly.
func (s *MarkerFacilityService) FetchRegionWaterInfo(latitude, longitude float64) (bool, error) {
	reqURL := fmt.Sprintf("%s?latitude=%f&longitude=%f&rapidapi-key=%s", s.Config.IsWaterURL, latitude, longitude, s.Config.IsWaterKEY)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp dto.WaterAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return false, fmt.Errorf("unmarshalling response: %w", err)
	}

	return apiResp.Water, nil
}

type CustomString string

func (cs *CustomString) UnmarshalJSON(data []byte) error {
	var asFloat float64
	if err := sonic.Unmarshal(data, &asFloat); err == nil {
		*cs = CustomString(fmt.Sprintf("%f", asFloat))
		return nil
	}

	var asString string
	if err := sonic.Unmarshal(data, &asString); err == nil {
		*cs = CustomString(asString)
		return nil
	}

	return errors.New("value must be a float or string")
}

type DataItem struct {
	Date          string       `json:"date"`
	ChulbongCount int          `json:"chulbong_count"`
	Longitude     CustomString `json:"longitude"`
	IsAble        int          `json:"is_able"`
	ID            string       `json:"id"`
	PyeongCount   int          `json:"pyeong_count"`
	Latitude      CustomString `json:"latitude"`
	Address       string       `json:"address,omitempty"`
}

func (s *MarkerFacilityService) FetchLatestMarkers(thresholdDate time.Time) ([]DataItem, error) {
	req, err := http.NewRequest(http.MethodGet, s.Config.CkURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data []DataItem
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	var filteredData []DataItem
	for _, item := range data {
		itemDate, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			continue
		}

		if itemDate.Equal(thresholdDate) || itemDate.After(thresholdDate) {
			filteredData = append(filteredData, item)
		}
	}

	return filteredData, nil
}

func (s *MarkerFacilityService) FetchRoadViewPicDate(latitude, longitude float64) (time.Time, error) {
	wcong := util.ConvertWGS84ToWCONGNAMUL(latitude, longitude)
	url := s.KakaoConfig.KakaoRoadViewAPI + "&PX=" + fmt.Sprintf("%f", wcong.X) + "&PY=" + fmt.Sprintf("%f", wcong.Y)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("User-Agent", userAgent)

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp kakao.StreetViewData
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return time.Time{}, fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(apiResp.StreetView.StreetList) == 0 {
		return time.Time{}, errors.New("no street view data available")
	}

	shotDateStr := apiResp.StreetView.StreetList[0].ShotDate
	shotDate, err := time.Parse("2006-01-02 15:04:05", shotDateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing shot date: %w", err)
	}

	return shotDate, nil
}
