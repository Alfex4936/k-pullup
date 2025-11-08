package providers

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
	_ "github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	_ "github.com/blevesearch/bleve/v2/analysis/char/html"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	_ "github.com/blevesearch/bleve/v2/analysis/token/ngram"
	_ "github.com/blevesearch/bleve/v2/analysis/token/unicodenorm"
	_ "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	_ "github.com/blevesearch/bleve/v2/index/upsidedown/store/boltdb"
)

// NewBleveIndex creates a new Bleve index service with sharding
func NewBleveIndex() ([]bleve.Index, bleve.Index, error) {
	var shards []bleve.Index
	searchShardHandler := bleve.NewIndexAlias()

	for i := 0; i < 3; i++ {
		indexShardName := fmt.Sprintf("markers_shard_%d.bleve", i)
		index, err := bleve.Open(indexShardName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open shard %d: %v", i, err)
		}
		shards = append(shards, index) // Store each shard
		searchShardHandler.Add(index)  // Add to alias for querying
	}

	return shards, searchShardHandler, nil
}
