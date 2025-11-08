package providers

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/tzf"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/contrib/websocket"
	"go.uber.org/zap"
	"fmt"
	"strconv"
	"strings"
)

// NewWsConfig creates WebSocket configuration
func NewWsConfig() websocket.Config {
	return websocket.Config{
		// Set the handshake timeout to a reasonable duration to prevent slowloris attacks.
		HandshakeTimeout: 5 * time.Second,

		// TODO: PRODUCTION
		Origins: []string{"https://test.k-pullup.com", "https://www.k-pullup.com", "https://m.k-pullup.com", "https://local.k-pullup.com:5173"},

		EnableCompression: true,

		RecoverHandler: func(c *websocket.Conn) {
			// Custom recover logic. By default, it logs the error and stack trace.
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "WebSocket panic: %v\n", r)
				debug.PrintStack()
				c.WriteMessage(websocket.CloseMessage, []byte{})
				c.Close()
			}
		},
	}
}

// NewStationData loads Korea station data from JSON file
func NewStationData() (map[string]dto.KoreaStation, error) {
	stationMap := make(map[string]dto.KoreaStation)

	file, err := os.Open("./resource/stations.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode the JSON data
	decoder := sonic.ConfigDefault.NewDecoder(file)
	var data struct {
		Data []struct {
			BldnNm string `json:"bldn_nm"`
			Lat    string `json:"lat"`
			Lot    string `json:"lot"`
		} `json:"DATA"`
	}
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	// Populate the stationMap
	for _, item := range data.Data {
		lat, err := strconv.ParseFloat(item.Lat, 64)
		if err != nil {
			continue
		}
		lon, err := strconv.ParseFloat(item.Lot, 64)
		if err != nil {
			continue
		}
		name := item.BldnNm

		// If the station name contains extra information in parentheses, strip it
		if idx := strings.Index(name, "("); idx != -1 {
			name = name[:idx]
		}

		// Ensure the station name ends with "역"
		if !strings.HasSuffix(name, "역") {
			name = name + "역"
		}

		stationMap[name] = dto.KoreaStation{
			Name:      name,
			Latitude:  lat,
			Longitude: lon,
		}
	}

	return stationMap, nil
}

// NewTimeZoneFinder loads timezone finder
func NewTimeZoneFinder(logger *zap.Logger) (tzf.F, error) {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		logger.Fatal("Error loading timezone finder", zap.Error(err))
		return &tzf.DefaultFinder{}, err
	}
	return finder, nil
}
