package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	sonic "github.com/bytedance/sonic"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestApplySearchMarkerSummaries(t *testing.T) {
	markers := []dto.ZincMarker{
		{MarkerID: 3, Address: "서울 <mark>강남구</mark>"},
		{MarkerID: 1, Address: "서울 강남구 역삼동"},
		{MarkerID: 2, Address: "서울 강남구 논현동"},
	}
	summaries := map[int]searchMarkerSummary{
		1: {MarkerID: 1, ThumbnailURL: strPtr("https://cdn/thumb1.webp"), PhotoCount: 3, FacilityCount: 2, FacilityTotal: 5},
		3: {MarkerID: 3, PhotoCount: 0, FacilityCount: 1, FacilityTotal: 4},
	}

	result := applySearchMarkerSummaries(markers, summaries)

	require.Len(t, result, 3)
	// relevance order and address (including highlight) are kept
	assert.Equal(t, []int{3, 1, 2}, []int{result[0].MarkerID, result[1].MarkerID, result[2].MarkerID})
	assert.Equal(t, "서울 <mark>강남구</mark>", result[0].Address)

	// marker with photos
	require.NotNil(t, result[1].ThumbnailURL)
	assert.Equal(t, "https://cdn/thumb1.webp", *result[1].ThumbnailURL)
	assert.Equal(t, 3, result[1].PhotoCount)
	assert.Equal(t, 2, result[1].FacilityCount)
	assert.Equal(t, 5, result[1].FacilityTotal)

	// marker without photos but with facilities
	assert.Nil(t, result[0].ThumbnailURL)
	assert.Equal(t, 0, result[0].PhotoCount)
	assert.Equal(t, 1, result[0].FacilityCount)
	assert.Equal(t, 4, result[0].FacilityTotal)

	// marker missing from DB (e.g. stale index) gets zero values
	assert.Nil(t, result[2].ThumbnailURL)
	assert.Zero(t, result[2].PhotoCount)
	assert.Zero(t, result[2].FacilityCount)
	assert.Zero(t, result[2].FacilityTotal)

	// input slice (shared with the search cache) is not mutated
	assert.Nil(t, markers[1].ThumbnailURL)
	assert.Zero(t, markers[1].PhotoCount)
}

func TestMarkerIDsOfDeduplicatesInOrder(t *testing.T) {
	ids := markerIDsOf([]dto.ZincMarker{{MarkerID: 5}, {MarkerID: 2}, {MarkerID: 5}, {MarkerID: 9}})
	assert.Equal(t, []int{5, 2, 9}, ids)
}

func TestSearchResponseJSONFields(t *testing.T) {
	thumb := "https://cdn/t.webp"
	resp := dto.MarkerSearchResponse{
		Took: 2,
		Markers: []dto.ZincMarker{
			{MarkerID: 1, Address: "a", ThumbnailURL: &thumb, PhotoCount: 1, FacilityCount: 2, FacilityTotal: 3},
			{MarkerID: 2, Address: "b"},
		},
	}

	b, err := sonic.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"markers": [
			{"markerId": 1, "address": "a", "thumbnailUrl": "https://cdn/t.webp", "photoCount": 1, "facilityCount": 2, "facilityTotal": 3},
			{"markerId": 2, "address": "b", "thumbnailUrl": null, "photoCount": 0, "facilityCount": 0, "facilityTotal": 0}
		],
		"took": 2
	}`, string(b))
}

func TestWithMarkerSummariesEmptyResult(t *testing.T) {
	// No DB access is needed for an empty result
	s := &BleveSearchService{Logger: zap.NewNop()}

	for _, markers := range [][]dto.ZincMarker{nil, {}} {
		resp := s.WithMarkerSummaries(dto.MarkerSearchResponse{Markers: markers, Took: 1})

		b, err := sonic.Marshal(resp)
		require.NoError(t, err)
		assert.JSONEq(t, `{"markers": [], "took": 1}`, string(b))
	}
}

func TestWithMarkerSummariesDBFailureKeepsResults(t *testing.T) {
	db, err := sqlx.Open("mysql", "user:pass@tcp(127.0.0.1:1)/none")
	require.NoError(t, err)
	require.NoError(t, db.Close()) // every query now fails

	s := &BleveSearchService{Logger: zap.NewNop(), DB: db}
	resp := s.WithMarkerSummaries(dto.MarkerSearchResponse{
		Markers: []dto.ZincMarker{{MarkerID: 1, Address: "a"}, {MarkerID: 2, Address: "b"}},
		Took:    4,
	})

	require.Len(t, resp.Markers, 2)
	assert.Equal(t, 4, resp.Took)
	assert.Equal(t, "a", resp.Markers[0].Address)
	assert.Nil(t, resp.Markers[0].ThumbnailURL)
	assert.Zero(t, resp.Markers[0].PhotoCount)
}

// TestFetchSearchMarkerSummariesMySQL runs the aggregation SQL against a real MySQL 8.
// Set SEARCH_SUMMARY_TEST_MYSQL_DSN to a DSN without a database name, e.g.
//
//	SEARCH_SUMMARY_TEST_MYSQL_DSN="root:pass@tcp(127.0.0.1:3306)/" go test ./service -run MySQL
//
// A throwaway database is created and dropped by the test.
func TestFetchSearchMarkerSummariesMySQL(t *testing.T) {
	dsn := os.Getenv("SEARCH_SUMMARY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SEARCH_SUMMARY_TEST_MYSQL_DSN not set")
	}

	admin, err := sqlx.Connect("mysql", dsn+"?parseTime=true")
	require.NoError(t, err)
	defer admin.Close()

	dbName := fmt.Sprintf("kpullup_search_summary_test_%d", time.Now().UnixNano())
	admin.MustExec("CREATE DATABASE " + dbName)
	defer admin.Exec("DROP DATABASE " + dbName)

	db, err := sqlx.Connect("mysql", dsn+dbName+"?parseTime=true&multiStatements=true")
	require.NoError(t, err)
	defer db.Close()

	db.MustExec(`
CREATE TABLE Markers (MarkerID INT PRIMARY KEY, Address VARCHAR(255));
CREATE TABLE Photos (
    PhotoID INT AUTO_INCREMENT PRIMARY KEY,
    MarkerID INT NOT NULL,
    PhotoURL VARCHAR(255),
    ThumbnailURL VARCHAR(255),
    UploadedAt DATETIME NOT NULL,
    INDEX (MarkerID)
);
CREATE TABLE MarkerFacilities (
    FacilityID INT NOT NULL,
    MarkerID INT NOT NULL,
    Quantity INT NOT NULL,
    INDEX (MarkerID)
);

INSERT INTO Markers (MarkerID, Address) VALUES (1, 'a'), (2, 'b'), (3, 'c'), (4, 'd');

-- marker 1: 3 photos x 3 facility rows (a naive JOIN would report 9 photos / doubled sums)
INSERT INTO Photos (MarkerID, PhotoURL, ThumbnailURL, UploadedAt) VALUES
    (1, 'https://s3/1-old.jpg', 'https://s3/1-old-thumb.webp', '2024-01-01 00:00:00'),
    (1, 'https://s3/1-new.jpg', 'https://s3/1-new-thumb.webp', '2024-03-01 00:00:00'),
    (1, 'https://s3/1-mid.jpg', NULL, '2024-02-01 00:00:00');
INSERT INTO MarkerFacilities (FacilityID, MarkerID, Quantity) VALUES (1, 1, 2), (2, 1, 3), (3, 1, 0);

-- marker 2: no photos, no facilities

-- marker 3: latest photo has no thumbnail -> PhotoURL fallback; same UploadedAt -> higher PhotoID wins
INSERT INTO Photos (MarkerID, PhotoURL, ThumbnailURL, UploadedAt) VALUES
    (3, 'https://s3/3-a.jpg', 'https://s3/3-a-thumb.webp', '2024-05-01 00:00:00'),
    (3, 'https://s3/3-b.jpg', '', '2024-05-01 00:00:00');

-- marker 4: facilities only, all with quantity 0
INSERT INTO MarkerFacilities (FacilityID, MarkerID, Quantity) VALUES (1, 4, 0), (2, 4, 0);
`)

	got, err := fetchSearchMarkerSummaries(t.Context(), db, []int{1, 2, 3, 4, 999})
	require.NoError(t, err)

	t.Run("photos and facilities are not multiplied by the join", func(t *testing.T) {
		m := got[1]
		require.NotNil(t, m.ThumbnailURL)
		assert.Equal(t, "https://s3/1-new-thumb.webp", *m.ThumbnailURL)
		assert.Equal(t, 3, m.PhotoCount)
		assert.Equal(t, 2, m.FacilityCount)
		assert.Equal(t, 5, m.FacilityTotal)
	})

	t.Run("marker without photos", func(t *testing.T) {
		m, ok := got[2]
		require.True(t, ok)
		assert.Nil(t, m.ThumbnailURL)
		assert.Zero(t, m.PhotoCount)
		assert.Zero(t, m.FacilityCount)
		assert.Zero(t, m.FacilityTotal)
	})

	t.Run("thumbnail falls back to PhotoURL with deterministic tie-break", func(t *testing.T) {
		m := got[3]
		require.NotNil(t, m.ThumbnailURL)
		assert.Equal(t, "https://s3/3-b.jpg", *m.ThumbnailURL)
		assert.Equal(t, 2, m.PhotoCount)
	})

	t.Run("zero-quantity facilities are not counted", func(t *testing.T) {
		assert.Zero(t, got[4].FacilityCount)
		assert.Zero(t, got[4].FacilityTotal)
	})

	t.Run("unknown marker is absent", func(t *testing.T) {
		_, ok := got[999]
		assert.False(t, ok)
	})

	t.Run("end to end keeps relevance order", func(t *testing.T) {
		s := &BleveSearchService{Logger: zap.NewNop(), DB: db}
		resp := s.WithMarkerSummaries(dto.MarkerSearchResponse{
			Markers: []dto.ZincMarker{{MarkerID: 3}, {MarkerID: 999}, {MarkerID: 1}, {MarkerID: 2}},
			Took:    7,
		})

		require.Len(t, resp.Markers, 4)
		assert.Equal(t, []int{3, 999, 1, 2}, []int{resp.Markers[0].MarkerID, resp.Markers[1].MarkerID, resp.Markers[2].MarkerID, resp.Markers[3].MarkerID})
		assert.Equal(t, 2, resp.Markers[0].PhotoCount)
		assert.Nil(t, resp.Markers[1].ThumbnailURL)
		assert.Equal(t, 3, resp.Markers[2].PhotoCount)
		assert.Nil(t, resp.Markers[3].ThumbnailURL)
		assert.Equal(t, 7, resp.Took)
	})
}

func strPtr(s string) *string { return &s }
