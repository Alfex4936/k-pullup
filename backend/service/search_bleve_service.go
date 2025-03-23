package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/Alfex4936/dkssud"
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	bleve_search "github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/jmoiron/sqlx"
	"go.uber.org/fx"
	"go.uber.org/zap"

	gocache "github.com/eko/gocache/lib/v4/cache"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
)

const (
	Analyzer     = "koCJKEdgeNgram"
	nearDistance = "2km"
)

var (
	// Map of Hangul initial consonant Unicode values to their corresponding Korean consonants.
	initialConsonantMap = map[rune]rune{
		0x1100: 'ㄱ', 0x1101: 'ㄲ', 0x1102: 'ㄴ', 0x1103: 'ㄷ', 0x1104: 'ㄸ',
		0x1105: 'ㄹ', 0x1106: 'ㅁ', 0x1107: 'ㅂ', 0x1108: 'ㅃ', 0x1109: 'ㅅ',
		0x110A: 'ㅆ', 0x110B: 'ㅇ', 0x110C: 'ㅈ', 0x110D: 'ㅉ', 0x110E: 'ㅊ',
		0x110F: 'ㅋ', 0x1110: 'ㅌ', 0x1111: 'ㅍ', 0x1112: 'ㅎ',
	}

	validInitialConsonants = map[rune]bool{
		'ㄱ': true, 'ㄲ': true, 'ㄴ': true, 'ㄷ': true, 'ㄸ': true,
		'ㄹ': true, 'ㅁ': true, 'ㅂ': true, 'ㅃ': true, 'ㅅ': true,
		'ㅆ': true, 'ㅇ': true, 'ㅈ': true, 'ㅉ': true, 'ㅊ': true,
		'ㅋ': true, 'ㅌ': true, 'ㅍ': true, 'ㅎ': true,
	}

	doubleConsonants = map[rune][]rune{
		'ㄳ': {'ㄱ', 'ㅅ'}, 'ㄵ': {'ㄴ', 'ㅈ'}, 'ㄶ': {'ㄴ', 'ㅎ'}, 'ㄺ': {'ㄹ', 'ㄱ'},
		'ㄻ': {'ㄹ', 'ㅁ'}, 'ㄼ': {'ㄹ', 'ㅂ'}, 'ㄽ': {'ㄹ', 'ㅅ'}, 'ㄾ': {'ㄹ', 'ㅌ'},
		'ㄿ': {'ㄹ', 'ㅍ'}, 'ㅀ': {'ㄹ', 'ㅎ'}, 'ㅄ': {'ㅂ', 'ㅅ'},
	}

	initials = "ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ"

	documentMatchPool = &sync.Pool{
		New: func() any {
			slice := make([]*bleve_search.DocumentMatch, 0, 100)
			return &slice
		},
	}

	termsPool = &sync.Pool{
		New: func() any {
			slice := make([]string, 0, 30)
			return &slice
		},
	}

	matchQueryPool = sync.Pool{
		New: func() any {
			return &query.MatchQuery{}
		},
	}

	prefixQueryPool = sync.Pool{
		New: func() any {
			return &query.PrefixQuery{}
		},
	}

	wildcardQueryPool = sync.Pool{
		New: func() any {
			return &query.WildcardQuery{}
		},
	}

	// matchPhraseQueryPool = sync.Pool{
	// 	New: func() any {
	// 		return &query.MatchPhraseQuery{}
	// 	},
	// }

	// boolQueryPool = sync.Pool{
	// 	New: func() any {
	// 		return &query.BooleanQuery{}
	// 	},
	// }

	// bufferPool = sync.Pool{
	// 	New: func() interface{} {
	// 		b := make([]byte, 0, 256)
	// 		return &b
	// 	},
	// }

	levenshtein = metrics.NewLevenshtein()
)

type BleveSearchService struct {
	Index  bleve.Index
	Shards []bleve.Index

	LocalCacheStorage *ristretto_store.RistrettoStore
	Logger            *zap.Logger
	DB                *sqlx.DB
	GetAllMarkersStmt *sqlx.Stmt

	searchCache *gocache.Cache[dto.MarkerSearchResponse]

	stationMap map[string]dto.KoreaStation

	batchPool   []*bleve.Batch
	batchLock   sync.Mutex
	pendingDocs uint32
}

func NewBleveSearchService(
	index bleve.Index, shards []bleve.Index,
	localCacheStorage *ristretto_store.RistrettoStore, logger *zap.Logger,
	db *sqlx.DB, stationMap map[string]dto.KoreaStation) *BleveSearchService {
	searchCache := gocache.New[dto.MarkerSearchResponse](localCacheStorage)

	getMarkerStmt, _ := db.Preparex("SELECT MarkerID, Address FROM Markers")
	levenshtein.CaseSensitive = false
	levenshtein.InsertCost = 1
	levenshtein.ReplaceCost = 2
	levenshtein.DeleteCost = 1

	return &BleveSearchService{Index: index, Shards: shards,
		searchCache: searchCache, Logger: logger, DB: db,
		GetAllMarkersStmt: getMarkerStmt, stationMap: stationMap,
		batchPool: make([]*bleve.Batch, len(shards)),
	}
}

func RegisteBleveLifecycle(lifecycle fx.Lifecycle, service *BleveSearchService) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return nil
		},
		OnStop: func(context.Context) error {
			service.GetAllMarkersStmt.Close()
			return nil
		},
	})
}

// SearchMarkerAddress calls bleve (Lucene-like) search
func (s *BleveSearchService) SearchMarkerAddress(t string) (dto.MarkerSearchResponse, error) {
	// t is already trimmed
	cacheKey := fmt.Sprintf("search:%s", t)
	cachedResponse, err := s.searchCache.Get(context.Background(), cacheKey)
	if err == nil {
		return cachedResponse, nil
	}

	// 쿼티 한글? -> 한글로 변환 (ex. "rudrleh" -> "경기도")
	if dkssud.IsQwertyHangul(t) {
		t = dkssud.QwertyToHangul(t)
	}

	// s.Logger.Info("Searching...", zap.String("query", t))

	response := dto.MarkerSearchResponse{Markers: make([]dto.ZincMarker, 0)}

	// Get a pointer to a slice from the pool
	termsPtr := termsPool.Get().(*[]string)
	terms := (*termsPtr)[:0] // Reset the slice

	// Split the search term by spaces and append to the pooled slice
	terms = append(terms, strings.Fields(t)...)
	if len(terms) == 0 {
		termsPool.Put(termsPtr)
		return response, nil
	}

	terms[0] = standardizeInitials(terms[0])

	// Channels to receive search results and the time taken
	resultsChan := make(chan *bleve_search.DocumentMatch, 100)
	tookTimesChan := make(chan time.Duration, 1)

	// Launch a single goroutine to perform the search
	go func() {
		performWholeQuerySearch(s.Index, t, terms, resultsChan, tookTimesChan, s.stationMap)
		close(resultsChan)
		close(tookTimesChan)
	}()

	// Get a pointer to a slice from the pool
	allResultsPtr := documentMatchPool.Get().(*[]*bleve_search.DocumentMatch)
	allResults := (*allResultsPtr)[:0] // Reset the slice
	var totalTook time.Duration

	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				resultsChan = nil
			} else if result != nil {
				allResults = append(allResults, result)
			}
		case took, ok := <-tookTimesChan:
			if !ok {
				tookTimesChan = nil
			} else {
				totalTook += took
			}
		}

		if resultsChan == nil && tookTimesChan == nil {
			break
		}
	}

	// Sort and ensure diverse results
	// diverseResults := ensureDiversity(allResults, 10)

	// Remove duplicates by keeping only the highest scoring result per document ID
	allResults = removeDuplicatesAndKeepHighestScore(allResults)

	if len(allResults) == 0 { // or if len <= 3?
		// If no results, try fuzzy search with controlled fuzziness
		fuzzyResults, fuzzyTook := performFuzzySearch(s.Index, terms)
		allResults = fuzzyResults
		totalTook += fuzzyTook
	}

	// If this is not a station search, adjust scores based on similarity and re-sort.
	if _, isStationSearch := s.stationMap[t]; !isStationSearch {
		adjustScoresBySimilarity(allResults, t)
		sortResultsByScore(allResults)
	}

	response.Took = int(totalTook.Milliseconds())
	response.Markers = extractMarkers(allResults)

	// Cache the response
	s.searchCache.Set(context.Background(), cacheKey, response)

	// Reset the slice and return the pointer to the pool
	*allResultsPtr = allResults[:0]
	documentMatchPool.Put(allResultsPtr)

	// Return terms slice to the pool
	*termsPtr = terms[:0]
	termsPool.Put(termsPtr)

	return response, nil
}

// TODO: use DAWG
func (s *BleveSearchService) AutoComplete(term string) ([]string, error) {
	var suggestions []string

	prefixQuery := bleve.NewPrefixQuery(term)
	searchRequest := bleve.NewSearchRequest(prefixQuery)
	searchRequest.Fields = []string{"fullAddress", "address", "province", "city"}
	searchRequest.Size = 10 // Limit the number of suggestions

	searchResult, err := s.Index.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("error performing autocomplete search: %v", err)
	}

	for _, hit := range searchResult.Hits {
		suggestions = append(suggestions, hit.Fields["fullAddress"].(string))
	}

	return suggestions, nil
}

func (s *BleveSearchService) InsertMarkerIndex(indexBody MarkerIndexData) error {
	// Compute which shard to use based on the marker ID (or any other key)
	shardIndex := indexBody.MarkerID % len(s.Shards)
	selectedShard := s.Shards[shardIndex]

	// Create or get batch for this shard
	s.batchLock.Lock()
	defer s.batchLock.Unlock()

	province, city, rest := splitAddress(indexBody.Address)
	indexBody.Province = province
	indexBody.City = city
	indexBody.FullAddress = indexBody.Address
	indexBody.Address = rest
	indexBody.InitialConsonants = ExtractInitialConsonants(indexBody.FullAddress)

	batch := s.batchPool[shardIndex]
	if batch == nil {
		batch = selectedShard.NewBatch()
		s.batchPool[shardIndex] = batch
	}

	// Add to batch
	err := batch.Index(strconv.Itoa(indexBody.MarkerID), indexBody)
	if err != nil {
		return fmt.Errorf("error adding to batch: %v", err)
	}

	// Track pending documents
	atomic.AddUint32(&s.pendingDocs, 1)

	// Auto-commit when batch size reaches
	if batch.Size() >= 5 {
		err = selectedShard.Batch(batch)
		if err != nil {
			atomic.AddUint32(&s.pendingDocs, -uint32(batch.Size())) // Rollback
			return err
		}
		atomic.AddUint32(&s.pendingDocs, -uint32(batch.Size()))
		s.batchPool[shardIndex] = nil // Reset batch
	}

	// Invalidate search cache
	s.InvalidateCache()

	defer s.Logger.Info("New Marker indexed", zap.Int("markerID", indexBody.MarkerID), zap.String("address", indexBody.FullAddress))

	return nil
}

func (s *BleveSearchService) DeleteMarkerIndex(markerId int) error {
	// Compute which shard to use based on the marker ID (same as in InsertMarkerIndex)
	shardIndex := markerId % len(s.Shards)
	selectedShard := s.Shards[shardIndex]

	// Perform the deletion operation on the selected shard
	err := selectedShard.Delete(strconv.Itoa(markerId))
	if err != nil {
		return fmt.Errorf("error deleting marker: %v", err)
	}

	// Invalidate search cache
	s.InvalidateCache()

	return nil
}

func (s *BleveSearchService) InvalidateCache() {
	s.searchCache.Clear(context.Background())
}

func (s *BleveSearchService) CheckIndexes() error {
	// Step 1: Fetch all valid marker IDs from the database along with their addresses
	markers, err := s.GetAllMarkers()
	if err != nil {
		return fmt.Errorf("error fetching valid marker IDs from database: %v", err)
	}

	// Convert marker data to a map for quick lookup by MarkerID
	markerDataMap := make(map[int]dto.MarkerIndexData, len(markers))
	for _, marker := range markers {
		markerDataMap[marker.MarkerID] = dto.MarkerIndexData{
			MarkerID: marker.MarkerID,
			Address:  marker.Address,
		}
	}

	// Step 2: Execute a MatchAllQuery to retrieve all documents from the index
	query := bleve.NewMatchAllQuery()
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = 10000

	searchResult, err := s.Index.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("error executing search: %v", err)
	}

	// Step 3: Iterate through all hits (documents) in the result
	for _, hit := range searchResult.Hits {
		markerID, err := strconv.Atoi(hit.ID)
		if err != nil {
			s.Logger.Error("Failed to convert document ID to markerID", zap.String("docID", hit.ID), zap.Error(err))
			continue
		}

		// Check if this marker exists in the database
		if _, existsInDB := markerDataMap[markerID]; !existsInDB {
			// Step 4: If the MarkerID doesn't exist in the database, delete it from the index
			err = s.DeleteMarkerIndex(markerID)
			if err != nil {
				s.Logger.Error("Failed to delete orphaned index", zap.Int("markerID", markerID), zap.Error(err))
			} else {
				s.Logger.Info("Orphaned index deleted", zap.Int("markerID", markerID))
			}
		}
	}

	// Step 5: Check if each valid marker from the database exists in the index, and index it if missing
	for markerID, markerData := range markerDataMap {
		exists, err := s.MarkerExists(markerID)
		if err != nil {
			s.Logger.Error("Error checking marker existence in index", zap.Int("markerID", markerID), zap.Error(err))
			continue
		}

		// If the marker doesn't exist in the index but is in the database, index it
		if !exists {
			err = s.InsertMarkerIndex(markerData)
			if err != nil {
				s.Logger.Error("Failed to index marker", zap.Int("markerID", markerID), zap.Error(err))
			} else {
				s.Logger.Info("Marker indexed", zap.Int("markerID", markerID))
			}
		}
	}

	return nil
}

// GetAllMarkers now returns a simplified list of markers
func (s *BleveSearchService) GetAllMarkers() ([]dto.MarkerOnlyWithAddr, error) {
	var markers []dto.MarkerOnlyWithAddr

	err := s.GetAllMarkersStmt.Select(&markers)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	// go s.MarkerLocationService.Redis.AddGeoMarkers(markers)

	return markers, nil
}

func (s *BleveSearchService) MarkerExists(markerID int) (bool, error) {
	// Iterate over all shards
	for _, shard := range s.Shards {
		// Fetch the document from the shard
		document, err := shard.Document(strconv.Itoa(markerID))
		if err != nil {
			return false, fmt.Errorf("error fetching document for markerID %d: %v", markerID, err)
		}

		// If the document is found, return true
		if document != nil {
			return true, nil
		}
	}

	// If no document was found in any shard, return false
	return false, nil
}

func (s *BleveSearchService) SearchMarkersNearLocation(t string) (dto.MarkerSearchResponse, error) {
	// Preprocess the search term
	s.Logger.Info("Preprocessed search term", zap.String("term", t))

	var lat, lon float64
	// Check if the term matches a station name
	if station, ok := s.stationMap[t]; ok {
		lat = station.Latitude
		lon = station.Longitude

		s.Logger.Info("Station name matches", zap.String("station", t), zap.Float64("lat", lat), zap.Float64("lon", lon))
	}

	response := dto.MarkerSearchResponse{Markers: make([]dto.ZincMarker, 0)}

	// Create a geo-distance query
	distance := "4km"
	geoQuery := bleve.NewGeoDistanceQuery(lon, lat, distance)
	geoQuery.SetField("coordinates")

	// Build the search request
	searchRequest := bleve.NewSearchRequestOptions(geoQuery, 15, 0, false)
	searchRequest.Fields = []string{"fullAddress", "coordinates"}
	searchRequest.SortBy([]string{"_score", "markerId"})

	// Perform the search
	searchResult, err := s.Index.Search(searchRequest)
	if err != nil {
		return response, err
	}

	// Process the search results
	response.Took = int(searchResult.Took.Milliseconds())
	response.Markers = extractMarkers(searchResult.Hits)

	return response, nil
}

func (s *BleveSearchService) FlushAllBatches() error {
	s.batchLock.Lock()
	defer s.batchLock.Unlock()

	totalFlushed := 0
	for i, batch := range s.batchPool {
		if batch != nil && batch.Size() > 0 {
			err := s.Shards[i].Batch(batch)
			if err != nil {
				return fmt.Errorf("shard %d: %v", i, err)
			}
			flushed := batch.Size()
			atomic.AddUint32(&s.pendingDocs, -uint32(flushed))
			totalFlushed += flushed
			s.batchPool[i] = nil
		}
	}
	s.Logger.Info("Flushed batches", zap.Int("count", totalFlushed))
	return nil
}

func (s *BleveSearchService) IndexAndRefresh(id int, data MarkerIndexData) error {
	err := s.InsertMarkerIndex(data)
	if err != nil {
		return err
	}

	// Force flush and refresh
	_ = s.FlushAllBatches()
	shardIndex := id % len(s.Shards)
	s.Shards[shardIndex].Close()
	newShard, _ := bleve.Open(fmt.Sprintf("markers_shard_%d.bleve", shardIndex))
	s.Shards[shardIndex] = newShard

	return nil
}

// func performSearch(index bleve.Index, term string, results chan<- *bleve_search.DocumentMatch, tookTimes chan<- time.Duration) {
// 	var queries []query.Query

// 	bufferPtr := bufferPool.Get().(*[]byte)
// 	defer bufferPool.Put(bufferPtr)
// 	buffer := (*bufferPtr)[:0]

// 	// Helper function to reset and reuse the buffer
// 	resetBuffer := func() {
// 		buffer = buffer[:0]
// 	}

// 	// Helper function to append string to buffer and get string back
// 	appendString := func(s string) string {
// 		resetBuffer()
// 		buffer = append(buffer, s...)
// 		return string(buffer)
// 	}

// 	// Exact match queries with higher boosts
// 	matchQueryFullAddress := matchQueryPool.Get().(*query.MatchQuery)
// 	defer matchQueryPool.Put(matchQueryFullAddress) // Put back into pool after use
// 	matchQueryFullAddress.SetField("fullAddress")
// 	matchQueryFullAddress.Analyzer = Analyzer
// 	matchQueryFullAddress.SetBoost(50.0)
// 	matchQueryFullAddress.Match = appendString(term)
// 	queries = append(queries, matchQueryFullAddress)

// 	// Phrase match query for exact phrases
// 	matchPhraseQueryFullAddress := matchPhraseQueryPool.Get().(*query.MatchPhraseQuery)
// 	defer matchPhraseQueryPool.Put(matchPhraseQueryFullAddress) // Put back into pool after use
// 	matchPhraseQueryFullAddress.SetField("fullAddress")
// 	matchPhraseQueryFullAddress.Analyzer = Analyzer
// 	matchPhraseQueryFullAddress.SetBoost(30.0)
// 	matchPhraseQueryFullAddress.MatchPhrase = appendString(term) // Correct usage of appendString
// 	queries = append(queries, matchPhraseQueryFullAddress)

// 	// Wildcard queries with lower boosts
// 	resetBuffer()
// 	buffer = append(buffer, '*')
// 	buffer = append(buffer, term...)
// 	buffer = append(buffer, '*')
// 	wildcardQueryFullAddress := wildcardQueryPool.Get().(*query.WildcardQuery)
// 	defer wildcardQueryPool.Put(wildcardQueryFullAddress) // Put back into pool after use
// 	wildcardQueryFullAddress.SetField("fullAddress")
// 	wildcardQueryFullAddress.SetBoost(10.0)
// 	wildcardQueryFullAddress.Wildcard = string(buffer)
// 	queries = append(queries, wildcardQueryFullAddress)

// 	prefixQueryFullAddress := prefixQueryPool.Get().(*query.PrefixQuery)
// 	defer prefixQueryPool.Put(prefixQueryFullAddress) // Put back into pool after use
// 	prefixQueryFullAddress.SetField("fullAddress")
// 	prefixQueryFullAddress.SetBoost(20.0)
// 	prefixQueryFullAddress.Prefix = appendString(term)
// 	queries = append(queries, prefixQueryFullAddress)

// 	// Additional fields and queries
// 	brokenConsonants := appendString(SegmentConsonants(term))

// 	matchQueryInitialConsonants := matchQueryPool.Get().(*query.MatchQuery)
// 	defer matchQueryPool.Put(matchQueryInitialConsonants) // Put back into pool after use
// 	matchQueryInitialConsonants.SetField("initialConsonants")
// 	matchQueryInitialConsonants.Analyzer = Analyzer
// 	matchQueryInitialConsonants.SetBoost(10.0)
// 	matchQueryInitialConsonants.Match = brokenConsonants // Correct usage of appendString previously done
// 	queries = append(queries, matchQueryInitialConsonants)

// 	resetBuffer()
// 	buffer = append(buffer, '*')
// 	buffer = append(buffer, brokenConsonants...)
// 	buffer = append(buffer, '*')

// 	wildcardQueryInitialConsonants := wildcardQueryPool.Get().(*query.WildcardQuery)
// 	defer wildcardQueryPool.Put(wildcardQueryInitialConsonants) // Put back into pool after use
// 	wildcardQueryInitialConsonants.SetField("initialConsonants")
// 	wildcardQueryInitialConsonants.SetBoost(7.0)
// 	wildcardQueryInitialConsonants.Wildcard = string(buffer)
// 	queries = append(queries, wildcardQueryInitialConsonants)

// 	prefixQueryInitialConsonants := prefixQueryPool.Get().(*query.PrefixQuery)
// 	defer prefixQueryPool.Put(prefixQueryInitialConsonants) // Put back into pool after use
// 	prefixQueryInitialConsonants.SetField("initialConsonants")
// 	prefixQueryInitialConsonants.SetBoost(12.0)
// 	prefixQueryInitialConsonants.Prefix = brokenConsonants
// 	queries = append(queries, prefixQueryInitialConsonants)

// 	standardizedProvince := appendString(standardizeProvince(term))
// 	if standardizedProvince != term {
// 		matchQueryProvince := prefixQueryPool.Get().(*query.PrefixQuery)
// 		defer prefixQueryPool.Put(matchQueryProvince) // Put back into pool after use
// 		matchQueryProvince.SetField("province")
// 		matchQueryProvince.SetBoost(8.0)
// 		matchQueryProvince.Prefix = standardizedProvince
// 		queries = append(queries, matchQueryProvince)
// 	} else {
// 		// Reuse queries for city, district, etc.
// 		prefixQueryCity := prefixQueryPool.Get().(*query.PrefixQuery)
// 		defer prefixQueryPool.Put(prefixQueryCity) // Put back into pool after use
// 		prefixQueryCity.SetField("city")
// 		prefixQueryCity.SetBoost(20.0)
// 		prefixQueryCity.Prefix = appendString(term)
// 		queries = append(queries, prefixQueryCity)

// 		matchQueryCity := matchQueryPool.Get().(*query.MatchQuery)
// 		defer matchQueryPool.Put(matchQueryCity) // Put back into pool after use
// 		matchQueryCity.SetField("city")
// 		matchQueryCity.Analyzer = Analyzer
// 		matchQueryCity.SetBoost(30.0)
// 		matchQueryCity.Match = appendString(term) // Correct usage of appendString
// 		queries = append(queries, matchQueryCity)

// 		prefixQueryAddr := prefixQueryPool.Get().(*query.PrefixQuery)
// 		defer prefixQueryPool.Put(prefixQueryAddr) // Put back into pool after use
// 		prefixQueryAddr.SetField("address")
// 		prefixQueryAddr.SetBoost(15.0)
// 		prefixQueryAddr.Prefix = appendString(term)
// 		queries = append(queries, prefixQueryAddr)

// 		resetBuffer()
// 		buffer = append(buffer, '*')
// 		buffer = append(buffer, term...)
// 		buffer = append(buffer, '*')
// 		wildcardQueryAddr := wildcardQueryPool.Get().(*query.WildcardQuery)
// 		defer wildcardQueryPool.Put(wildcardQueryAddr) // Put back into pool after use
// 		wildcardQueryAddr.SetField("address")
// 		wildcardQueryAddr.SetBoost(7.0)
// 		wildcardQueryAddr.Wildcard = string(buffer)
// 		queries = append(queries, wildcardQueryAddr)
// 	}

// 	disjunctionQuery := bleve.NewDisjunctionQuery(queries...)
// 	searchRequest := bleve.NewSearchRequestOptions(disjunctionQuery, 10, 0, false)
// 	searchRequest.Fields = []string{"fullAddress", "address", "province", "city", "initialConsonants"}
// 	searchRequest.Size = 10
// 	searchRequest.Highlight = bleve.NewHighlightWithStyle("html")
// 	searchRequest.SortBy([]string{"_score", "markerId"})

// 	searchResult, err := index.Search(searchRequest)
// 	if err != nil {
// 		return
// 	}

// 	tookTimes <- searchResult.Took
// 	for _, hit := range searchResult.Hits {
// 		results <- hit
// 	}
// }

func performWholeQuerySearch(index bleve.Index, t string, terms []string, results chan<- *bleve_search.DocumentMatch, tookTimes chan<- time.Duration, stationMap map[string]dto.KoreaStation) {
	// Pre-process terms to assign them to fields
	termAssignments := assignTermsToFields(terms)

	// Create a BooleanQuery
	boolQuery := bleve.NewBooleanQuery()

	// Add a MatchPhraseQuery on fullAddress for the entire search term with high boost
	matchPhraseQuery := bleve.NewMatchPhraseQuery(t)
	matchPhraseQuery.SetField("fullAddress")
	matchPhraseQuery.SetBoost(300.0) // Higher boost for exact phrase
	boolQuery.AddShould(matchPhraseQuery)

	var perTermDisjunctions []query.Query
	var provinceTerm, cityTerm string
	hasProvince := false
	hasCity := false
	isStation := false
	var stationLng, stationLat float64

	for _, assignment := range termAssignments {
		var termQueries []query.Query

		termStr := assignment.Term
		field := assignment.Field

		var matchBoost, prefixBoost float64

		switch field {
		case "province":
			matchBoost = 200.0
			prefixBoost = 180.0
			hasProvince = true
			provinceTerm = termStr
		case "city":
			matchBoost = 180.0
			prefixBoost = 160.0
			hasCity = true
			cityTerm = termStr
		case "address":
			matchBoost = 130.0
			prefixBoost = 110.0
		case "initialConsonants":
			matchBoost = 30.0
			prefixBoost = 20.0
			termStr = SegmentConsonants(termStr)
		default:
			matchBoost = 100.0
			prefixBoost = 80.0
		}

		// Check if the term matches a station name
		if station, ok := stationMap[t]; ok {
			// geo-distance query
			geoQuery := bleve.NewGeoDistanceQuery(station.Longitude, station.Latitude, nearDistance)
			geoQuery.SetField("coordinates")

			termQueries = append(termQueries, geoQuery)

			isStation = true
			stationLng = station.Longitude
			stationLat = station.Latitude
		}

		// MatchQuery for the assigned field
		matchQuery := matchQueryPool.Get().(*query.MatchQuery)
		*matchQuery = query.MatchQuery{Match: termStr}
		matchQuery.SetField(field)
		matchQuery.SetBoost(matchBoost)
		termQueries = append(termQueries, matchQuery)

		// PrefixQuery for the assigned field
		prefixQuery := prefixQueryPool.Get().(*query.PrefixQuery)
		*prefixQuery = query.PrefixQuery{Prefix: termStr}
		prefixQuery.SetField(field)
		prefixQuery.SetBoost(prefixBoost)
		termQueries = append(termQueries, prefixQuery)

		// WildcardQuery for the assigned field (optional)
		wildcardQuery := wildcardQueryPool.Get().(*query.WildcardQuery)
		*wildcardQuery = query.WildcardQuery{Wildcard: "*" + termStr + "*"}
		wildcardQuery.SetField(field)
		wildcardQuery.SetBoost(prefixBoost / 2) // Lower boost for wildcard
		termQueries = append(termQueries, wildcardQuery)

		// Include matching in fullAddress with lower boosts
		matchQueryFullAddress := matchQueryPool.Get().(*query.MatchQuery)
		*matchQueryFullAddress = query.MatchQuery{Match: termStr}
		matchQueryFullAddress.SetField("fullAddress")
		matchQueryFullAddress.SetBoost(70.0)
		termQueries = append(termQueries, matchQueryFullAddress)

		prefixQueryFullAddress := prefixQueryPool.Get().(*query.PrefixQuery)
		*prefixQueryFullAddress = query.PrefixQuery{Prefix: termStr}
		prefixQueryFullAddress.SetField("fullAddress")
		prefixQueryFullAddress.SetBoost(50.0)
		termQueries = append(termQueries, prefixQueryFullAddress)

		// Combine the term queries into a DisjunctionQuery (logical OR)
		termDisjunction := bleve.NewDisjunctionQuery()
		termDisjunction.Disjuncts = append(termDisjunction.Disjuncts, termQueries...)

		perTermDisjunctions = append(perTermDisjunctions, termDisjunction)

		// After using the queries, reset and put them back to the pool
		defer func(q1 *query.MatchQuery, q2 *query.PrefixQuery, q3 *query.WildcardQuery, q4 *query.MatchQuery, q5 *query.PrefixQuery) {
			*q1 = query.MatchQuery{}
			matchQueryPool.Put(q1)

			*q2 = query.PrefixQuery{}
			prefixQueryPool.Put(q2)

			*q3 = query.WildcardQuery{}
			wildcardQueryPool.Put(q3)

			*q4 = query.MatchQuery{}
			matchQueryPool.Put(q4)

			*q5 = query.PrefixQuery{}
			prefixQueryPool.Put(q5)
		}(matchQuery, prefixQuery, wildcardQuery, matchQueryFullAddress, prefixQueryFullAddress)
	}

	// Combine all per-term disjunctions into a ConjunctionQuery (logical AND)
	overallConjunction := bleve.NewConjunctionQuery(perTermDisjunctions...)
	boolQuery.AddMust(overallConjunction)

	// If both province and city are present, boost documents where both terms match
	if hasProvince && hasCity {
		combinedMatchQuery := bleve.NewBooleanQuery()

		// Match province
		provinceMatch := matchQueryPool.Get().(*query.MatchQuery)
		*provinceMatch = query.MatchQuery{Match: provinceTerm}
		provinceMatch.SetField("province")
		provinceMatch.SetBoost(500.0) // High boost for matching province
		combinedMatchQuery.AddMust(provinceMatch)

		// Match city
		cityMatch := matchQueryPool.Get().(*query.MatchQuery)
		*cityMatch = query.MatchQuery{Match: cityTerm}
		cityMatch.SetField("city")
		cityMatch.SetBoost(500.0) // High boost for matching city
		combinedMatchQuery.AddMust(cityMatch)

		// Combine into a boosting query
		combinedShouldQuery := bleve.NewBooleanQuery()
		combinedShouldQuery.AddMust(combinedMatchQuery)
		combinedShouldQuery.SetBoost(800.0) // Extra boost for matching both

		// Add to the main query
		boolQuery.AddShould(combinedShouldQuery)

		// After using the queries, reset and put them back to the pool
		defer func(q1 *query.MatchQuery, q2 *query.MatchQuery) {
			*q1 = query.MatchQuery{}
			matchQueryPool.Put(q1)

			*q2 = query.MatchQuery{}
			matchQueryPool.Put(q2)
		}(provinceMatch, cityMatch)
	}

	// Build the search request
	searchRequest := bleve.NewSearchRequestOptions(boolQuery, 15, 0, false)
	searchRequest.Fields = []string{"fullAddress", "address", "province", "city", "initialConsonants"}
	searchRequest.Size = 15
	searchRequest.Highlight = bleve.NewHighlightWithStyle("html")

	if isStation {
		// TODO: Gotta update the main function to know not sort
		sortGeo, _ := search.NewSortGeoDistance("coordinates", "m", stationLng, stationLat, false)
		searchRequest.SortByCustom(search.SortOrder{sortGeo})
	} else {
		searchRequest.SortBy([]string{"-_score", "markerId"}) // Sort by descending score
	}

	searchResult, err := index.Search(searchRequest)
	if err != nil {
		return
	}

	// resultJSON, err := sonic.MarshalIndent(searchResult, "", "  ")
	// if err != nil {
	// 	log.Fatalf("Failed to marshal search result: %v", err)
	// }
	// log.Println(string(resultJSON))

	// for _, hit := range searchResult.Hits {
	// 	if len(hit.Sort) > 0 {
	// 		// Decode binary-encoded float64 from the sort value
	// 		sortVal := hit.Sort[0]
	// 		if len(sortVal) >= 8 { // float64 needs 8 bytes
	// 			bits := binary.BigEndian.Uint64([]byte(sortVal[:8]))
	// 			distance := math.Float64frombits(bits)
	// 			log.Printf("Hit %s, Distance: %.2f meters", hit.ID, distance)
	// 		} else {
	// 			log.Printf("Hit %s, sort value too short: %v", hit.ID, sortVal)
	// 		}
	// 	}
	// }

	tookTimes <- searchResult.Took
	for _, hit := range searchResult.Hits {
		results <- hit
	}
}

// Perform search with facets
func performSearchFacet(index bleve.Index, term string, results chan<- *bleve_search.DocumentMatch, tookTimes chan<- time.Duration) {
	var queries []query.Query

	// Exact match queries with higher boosts
	matchQueryFullAddress := query.NewMatchQuery(term)
	matchQueryFullAddress.SetField("fullAddress")
	matchQueryFullAddress.Analyzer = "koCJKEdgeNgram"
	matchQueryFullAddress.SetBoost(25.0)
	queries = append(queries, matchQueryFullAddress)

	// Phrase match query for exact phrases
	matchPhraseQueryFullAddress := query.NewMatchPhraseQuery(term)
	matchPhraseQueryFullAddress.SetField("fullAddress")
	matchPhraseQueryFullAddress.Analyzer = "koCJKEdgeNgram"
	matchPhraseQueryFullAddress.SetBoost(20.0)
	queries = append(queries, matchPhraseQueryFullAddress)

	// Wildcard queries with lower boosts
	wildcardQueryFullAddress := query.NewWildcardQuery("*" + term + "*")
	wildcardQueryFullAddress.SetField("fullAddress")
	wildcardQueryFullAddress.SetBoost(10.0)
	queries = append(queries, wildcardQueryFullAddress)

	// Additional fields and queries
	brokenConsonants := SegmentConsonants(term)
	matchQueryInitialConsonants := query.NewMatchQuery(brokenConsonants)
	matchQueryInitialConsonants.SetField("initialConsonants")
	matchQueryInitialConsonants.Analyzer = "koCJKEdgeNgram"
	matchQueryInitialConsonants.SetBoost(15.0)
	queries = append(queries, matchQueryInitialConsonants)

	wildcardQueryInitialConsonants := query.NewWildcardQuery("*" + brokenConsonants + "*")
	wildcardQueryInitialConsonants.SetField("initialConsonants")
	wildcardQueryInitialConsonants.SetBoost(5.0)
	queries = append(queries, wildcardQueryInitialConsonants)

	standardizedProvince := standardizeProvince(term)
	if standardizedProvince != term {
		matchQueryProvince := query.NewMatchQuery(standardizedProvince)
		matchQueryProvince.SetField("province")
		matchQueryProvince.Analyzer = "koCJKEdgeNgram"
		matchQueryProvince.SetBoost(1.5)
		queries = append(queries, matchQueryProvince)
	} else {
		prefixQueryCity := query.NewPrefixQuery(term)
		prefixQueryCity.SetField("city")
		prefixQueryCity.SetBoost(10.0)
		queries = append(queries, prefixQueryCity)

		matchQueryCity := query.NewMatchQuery(term)
		matchQueryCity.SetField("city")
		matchQueryCity.Analyzer = "koCJKEdgeNgram"
		matchQueryCity.SetBoost(10.0)
		queries = append(queries, matchQueryCity)

		matchQueryDistrict := query.NewMatchQuery(term)
		matchQueryDistrict.SetField("district")
		matchQueryDistrict.Analyzer = "koCJKEdgeNgram"
		matchQueryDistrict.SetBoost(5.0)
		queries = append(queries, matchQueryDistrict)

		prefixQueryAddr := query.NewPrefixQuery(term)
		prefixQueryAddr.SetField("address")
		prefixQueryAddr.SetBoost(5.0)
		queries = append(queries, prefixQueryAddr)

		wildcardQueryAddr := query.NewWildcardQuery("*" + term + "*")
		wildcardQueryAddr.SetField("address")
		wildcardQueryAddr.SetBoost(2.0)
		queries = append(queries, wildcardQueryAddr)
	}

	disjunctionQuery := bleve.NewDisjunctionQuery(queries...)
	searchRequest := bleve.NewSearchRequest(disjunctionQuery)
	searchRequest.Highlight = bleve.NewHighlightWithStyle("html")
	searchRequest.Fields = []string{"fullAddress", "address", "province", "city", "initialConsonants"}
	searchRequest.Size = 10
	searchRequest.SortBy([]string{"_score", "markerId"})

	// Add facet request for province
	facetRequest := bleve.NewFacetRequest("province", 10)
	searchRequest.AddFacet("province_facets", facetRequest)

	searchResult, err := index.Search(searchRequest)
	if err != nil {
		log.Printf("Error performing search: %v", err)
		return
	}

	tookTimes <- searchResult.Took
	for _, hit := range searchResult.Hits {
		results <- hit
	}

	// Print facet results
	if facet, found := searchResult.Facets["province_facets"]; found {
		fmt.Printf("Facet Results for 'province':\n")
		fmt.Printf("Total: %d\n", facet.Total)
		fmt.Printf("Missing: %d\n", facet.Missing)
		for _, term := range facet.Terms.Terms() {
			fmt.Printf("Term: %s, Count: %d\n", term.Term, term.Count)
		}
	}
}

func performFuzzySearch(index bleve.Index, terms []string) ([]*bleve_search.DocumentMatch, time.Duration) {
	var allResults []*bleve_search.DocumentMatch
	var totalTook time.Duration

	for _, term := range terms {
		fuzzyQuery := bleve.NewFuzzyQuery(term)
		fuzzyQuery.Fuzziness = 1
		searchRequest := bleve.NewSearchRequest(fuzzyQuery)
		searchRequest.Fields = []string{"fullAddress", "address", "province", "city", "initialConsonants"}
		searchRequest.Size = 10
		searchRequest.Highlight = bleve.NewHighlightWithStyle("html")
		searchRequest.SortBy([]string{"-_score", "markerId"})
		searchResult, err := index.Search(searchRequest)
		if err != nil {
			log.Printf("Error fuzzy searching marker: %v", err)
			continue
		}
		allResults = append(allResults, searchResult.Hits...)
		totalTook += searchResult.Took
	}

	return allResults, totalTook
}

func extractMarkers(allResults []*bleve_search.DocumentMatch) []dto.ZincMarker {
	markers := make([]dto.ZincMarker, 0, len(allResults))
	for _, hit := range allResults {
		intID, _ := strconv.Atoi(hit.ID)
		var address string
		if fragments, ok := hit.Fragments["fullAddress"]; ok && len(fragments) > 0 {
			address = fragments[0]
		} else {
			address = hit.Fields["fullAddress"].(string)
		}
		markers = append(markers, dto.ZincMarker{
			MarkerID: intID,
			Address:  address,
		})
	}
	return markers
}

// extractInitialConsonants extracts the initial consonants from a Korean string.
//
// ex) "부산 해운대구 좌동 1395" -> "ㅂㅅㅎㅇㄷㄱㅈㄷ"
func ExtractInitialConsonants(s string) string {
	var initials []rune
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			initial := (r - 0xAC00) / 28 / 21
			if mapped, exists := initialConsonantMap[0x1100+initial]; exists {
				initials = append(initials, mapped)
			}
		}
	}
	return string(initials)
}

// Split the user input into valid Korean initial consonants, breaking double consonants where necessary
//
// ex) "앍돍ㅄㄳ산" -> "앍돍ㅂㅅㄱㅅ산"
func SegmentConsonants(input string) string {
	var result []rune

	for _, r := range input {
		// Check if the character is a valid initial consonant
		if validInitialConsonants[r] {
			result = append(result, r)
		} else if components, found := doubleConsonants[r]; found {
			// If it's a double consonant, break it into its components
			result = append(result, components...)
		} else {
			// If it's not a valid consonant or double consonant, add it as is
			result = append(result, r)
		}
	}

	return string(result)
}

func standardizeProvince(province string) string {
	switch province {
	case "경기", "경기도", "ㄱㄱㄷ":
		return "경기도"
	case "서울", "서울특별시", "ㅅㅇ", "ㅅㅇㅌㅂㅅ":
		return "서울특별시"
	case "부산", "부산광역시", "ㅄ":
		return "부산광역시"
	case "대구", "대구광역시", "ㄷㄱ":
		return "대구광역시"
	case "인천", "인천광역시", "ㅇㅊ":
		return "인천광역시"
	case "제주", "제주특별자치도", "제주도", "ㅈㅈㄷ":
		return "제주특별자치도"
	case "대전", "대전광역시":
		return "대전광역시"
	case "울산", "울산광역시":
		return "울산광역시"
	case "광주", "광주광역시":
		return "광주광역시"
	case "세종", "세종특별자치시":
		return "세종특별자치시"
	case "강원", "강원도", "강원특별자치도", "ㄱㅇㄷ":
		return "강원특별자치도"
	case "경남", "경상남도":
		return "경상남도"
	case "경북", "경상북도":
		return "경상북도"
	case "전북", "전북특별자치도":
		return "전북특별자치도"
	case "충남", "충청남도":
		return "충청남도"
	case "충북", "충청북도":
		return "충청북도"
	case "전남", "전라남도":
		return "전라남도"
	default:
		return province
	}
}

func standardizeInitials(initials string) string {
	switch initials {
	case "ㄱㄱ":
		return "ㄱㄱㄷ"
	case "ㅅㅇ", "ㅅㅇㅅ":
		return "ㅅㅇㅌㅂㅅ"
	case "ㅄ", "ㅄㅅ", "ㅂㅅ":
		return "ㅂㅅㄱㅇㅅ"
	case "ㄷㄱ":
		return "ㄷㄱㄱㅇㅅ"
	case "ㅇㅊㅅ":
		return "ㅇㅊㄱㅇㅅ"
	case "ㅈㅈㄷ", "ㅈㅈ":
		return "ㅈㅈㅌㅂㅈㅊㄷ"
	case "ㄷㅈ":
		return "ㄷㅈㄱㅇㅅ"
	case "ㅇㅅ":
		return "ㅇㅅㄱㅇㅅ"
	case "ㄱㅈ":
		return "ㄱㅈㄱㅇㅅ"
	case "ㅅㅈㅅ":
		return "ㅅㅈㅌㅂㅈㅊㅅ"
	case "ㄱㅇㄷ":
		return "ㄱㅇㅌㅂㅈㅊㄷ"
	case "ㄳㄴㄷ", "ㄱㅅㄴㄷ":
		return "ㄱㅅㄴㄷ"
	case "ㄳㅂㄷ", "ㄱㅅㅂㄷ":
		return "ㄱㅅㅂㄷ"
	case "ㅈㅂ":
		return "ㅈㅂㅌㅂㅈㅊㄷ"
	case "ㅊㄴ":
		return "ㅊㅊㄴㄷ"
	case "ㅊㅂ":
		return "ㅊㅊㅂㄷ"
	case "ㅈㄴ":
		return "ㅈㄹㄴㄷ"
	default:
		return initials
	}
}

func splitAddress(address string) (string, string, string) {
	parts := strings.Fields(address)
	if len(parts) < 2 {
		return "", "", address
	}
	province := standardizeProvince(parts[0])
	city := parts[1]
	rest := strings.Join(parts[2:], " ")
	return province, city, rest
}

func ensureDiversity(results []*bleve_search.DocumentMatch, limit int) []*bleve_search.DocumentMatch {
	addressSet := make(map[string]struct{})
	var diverseResults []*bleve_search.DocumentMatch

	for _, result := range results {
		if len(diverseResults) >= limit {
			break
		}
		address := result.Fields["fullAddress"].(string)
		if _, exists := addressSet[address]; !exists {
			addressSet[address] = struct{}{}
			diverseResults = append(diverseResults, result)
		}
	}

	return diverseResults
}

func adjustScoresBySimilarity(allResults []*bleve_search.DocumentMatch, query string) {
	for _, result := range allResults {
		address := result.Fields["fullAddress"].(string)

		// Calculate similarity between the query and the address
		similarity := strutil.Similarity(query, address, levenshtein)

		// Adjust the result score by adding the similarity score
		if similarity > 0.5 {
			result.Score += similarity * 0.5
		} else {
			result.Score -= similarity * 0.5

		}
	}
}

func sortResultsByScore(results []*bleve_search.DocumentMatch) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}

func removeDuplicatesAndKeepHighestScore(results []*bleve_search.DocumentMatch) []*bleve_search.DocumentMatch {
	// Map to store the highest scoring DocumentMatch for each unique document ID
	docMap := make(map[string]*bleve_search.DocumentMatch)

	// Iterate through the results and keep the highest score for each document ID
	for _, result := range results {
		if existing, found := docMap[result.ID]; found {
			// If the document ID exists, compare the scores and keep the higher one
			if result.Score > existing.Score {
				docMap[result.ID] = result
			}
		} else {
			// If the document ID is not in the map, add it
			docMap[result.ID] = result
		}
	}

	// Convert the map back to a slice
	uniqueResults := make([]*bleve_search.DocumentMatch, 0, len(docMap))
	for _, result := range docMap {
		uniqueResults = append(uniqueResults, result)
	}

	return uniqueResults
}

type TermAssignment struct {
	Term  string
	Field string
}

func assignTermsToFields(terms []string) []TermAssignment {
	assignments := make([]TermAssignment, 0, len(terms))

	for _, term := range terms {
		if dkssud.IsQwertyHangul(term) {
			term = dkssud.QwertyToHangul(term)
		}

		if util.IsProvince(term) {
			assignments = append(assignments, TermAssignment{Term: term, Field: "province"})
		} else if util.IsCity(term) {
			assignments = append(assignments, TermAssignment{Term: term, Field: "city"})
		} else if isNumeric(term) {
			assignments = append(assignments, TermAssignment{Term: term, Field: "address"})
		} else if isInitialConsonant(term) {
			assignments = append(assignments, TermAssignment{Term: term, Field: "initialConsonants"})
		} else {
			assignments = append(assignments, TermAssignment{Term: term, Field: "address"})
		}
	}

	return assignments
}

// Check if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Check if a string consists of only Korean initial consonants
func isInitialConsonant(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.Hangul, r) && !strings.ContainsRune(initials, r) {
			return false
		}
	}
	return true
}

// Preprocess the search term to remove common suffixes
func preprocessSearchTerm(term string) string {
	commonSuffixes := []string{"역", "동", "시", "구", "군", "읍", "면", "리"}
	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(term, suffix) {
			return strings.TrimSuffix(term, suffix)
		}
	}
	return term
}
