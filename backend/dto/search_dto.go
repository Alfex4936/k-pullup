package dto

// ElasticsearchResponse represents the entire JSON response structure
type ElasticsearchResponse struct {
	Hits     HitsData `json:"hits"`
	Shards   Shards   `json:"_shards"`
	Took     int      `json:"took"`
	TimedOut bool     `json:"timed_out"`
}

// Shards struct represents the "_shards" JSON object
type Shards struct {
	Total      int `json:"total"`
	Successful int `json:"successful"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

// HitsData struct represents the "hits" JSON object
type HitsData struct {
	MaxScore float64     `json:"max_score"`
	Total    TotalValue  `json:"total"`
	Hits     []HitDetail `json:"hits"`
}

// TotalValue struct captures the nested "total" JSON object within "hits"
type TotalValue struct {
	Value int `json:"value"`
}

// HitDetail struct represents each element in the "hits" array
type HitDetail struct {
	Score     float64    `json:"_score"`
	Index     string     `json:"_index"`
	Type      string     `json:"_type"`
	ID        string     `json:"_id"`
	Timestamp string     `json:"@timestamp"`
	Source    ZincMarker `json:"_source"`
}

// ZincMarker struct represents the "_source" nested JSON object within each "hit"
type ZincMarker struct {
	Timestamp string `json:"@timestamp,omitempty"`
	Address   string `json:"address"`
	Highlight string `json:"highlight,omitempty"`
	MarkerID  int    `json:"markerId"`

	// Search result card summary (filled from DB after the index search, not stored in the index)
	ThumbnailURL  *string `json:"thumbnailUrl"` // null when the marker has no photo
	PhotoCount    int     `json:"photoCount"`
	FacilityCount int     `json:"facilityCount"` // facility kinds with quantity >= 1
	FacilityTotal int     `json:"facilityTotal"` // sum of all facility quantities
}

// FuzzSearch represents the structure of the search
type FuzzMarkerSearch struct {
	Query        Query    `json:"query"`
	SourceFields []string `json:"_source"`
	SearchType   string   `json:"search_type"`
	From         int      `json:"from"`
	MaxResults   int      `json:"max_results"`
}

// Query represents the "query" part of the search request
type Query struct {
	Term string `json:"term"`
}

type MarkerSearchResponse struct {
	Markers []ZincMarker `json:"markers"`
	Took    int          `json:"took"`
}

type MarkerIndexData struct {
	MarkerID          int    `json:"markerId"`
	Province          string `json:"province"`
	City              string `json:"city"`
	Address           string `json:"address"` // such as Korean: 경기도 부천시 소사구 경인로29번길 32, 우성아파트
	FullAddress       string `json:"fullAddress"`
	InitialConsonants string `json:"initialConsonants"` // 초성
}

type KoreaStation struct {
	Name      string
	Latitude  float64
	Longitude float64
}
