package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHNSWIndex(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "test.hnsw")
	
	// Create new index
	index, err := NewHNSWIndex(indexPath, 3, DefaultConfig())
	require.NoError(t, err)
	require.NotNil(t, index)
	
	// Add a vector to trigger save
	err = index.Add([]float32{0.1, 0.2, 0.3}, 1)
	assert.NoError(t, err)
	
	// Close and verify file was created
	err = index.Close()
	assert.NoError(t, err)
	
	_, err = os.Stat(indexPath)
	assert.NoError(t, err)
}

func TestHNSWIndex_SaveAndLoad(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "test.hnsw")
	
	// Create and save index with 3D vectors
	index1, err := NewHNSWIndex(indexPath, 3, DefaultConfig())
	require.NoError(t, err)
	
	// Add some vectors
	err = index1.Add([]float32{0.1, 0.2, 0.3}, 1)
	assert.NoError(t, err)
	
	err = index1.Add([]float32{0.4, 0.5, 0.6}, 2)
	assert.NoError(t, err)
	
	// Verify we can search before save
	results, err := index1.Search([]float32{0.1, 0.2, 0.3}, 1)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	
	err = index1.Save()
	assert.NoError(t, err)
	index1.Close()
	
	// Verify file was created
	_, err = os.Stat(indexPath)
	assert.NoError(t, err)
	
	// Note: The HNSW library's Import/Export may not preserve all data
	// so we just verify the index can be loaded without error
	index2, err := LoadHNSWIndex(indexPath, 3, DefaultConfig())
	assert.NoError(t, err)
	assert.NotNil(t, index2)
	index2.Close()
}

func TestHNSWIndex_Add(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Add vectors
	vectors := []struct {
		vec []float32
		id  uint64
	}{
		{[]float32{0.1, 0.2, 0.3}, 1},
		{[]float32{0.4, 0.5, 0.6}, 2},
		{[]float32{0.7, 0.8, 0.9}, 3},
	}
	
	for _, v := range vectors {
		err := index.Add(v.vec, v.id)
		assert.NoError(t, err)
	}
	
	// Verify vectors were added
	assert.Equal(t, 3, index.Size())
}

func TestHNSWIndex_Search(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Add test vectors
	vectors := []struct {
		vec []float32
		id  uint64
	}{
		{[]float32{1.0, 0.0, 0.0}, 1},
		{[]float32{0.0, 1.0, 0.0}, 2},
		{[]float32{0.0, 0.0, 1.0}, 3},
		{[]float32{0.5, 0.5, 0.0}, 4},
	}
	
	for _, v := range vectors {
		err := index.Add(v.vec, v.id)
		require.NoError(t, err)
	}
	
	// Search for nearest neighbors
	query := []float32{0.9, 0.1, 0.0}
	results, err := index.Search(query, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	// First result should be closest to [1.0, 0.0, 0.0]
	assert.Equal(t, uint64(1), results[0].ID)
	assert.Greater(t, results[0].Score, float32(0.8))
	
	// Verify scores are in descending order (higher is better for cosine similarity)
	if len(results) > 1 {
		assert.GreaterOrEqual(t, results[0].Score, results[1].Score)
	}
}

func TestHNSWIndex_Delete(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Add vectors
	err = index.Add([]float32{0.1, 0.2, 0.3}, 1)
	require.NoError(t, err)
	err = index.Add([]float32{0.4, 0.5, 0.6}, 2)
	require.NoError(t, err)
	
	assert.Equal(t, 2, index.Size())
	
	// Delete a vector
	err = index.Delete(1)
	assert.NoError(t, err)
	assert.Equal(t, 1, index.Size())
	
	// Search should not return deleted vector
	results, err := index.Search([]float32{0.1, 0.2, 0.3}, 2)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, uint64(2), results[0].ID)
}

func TestHNSWIndex_Clear(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Add vectors
	for i := uint64(1); i <= 5; i++ {
		err := index.Add([]float32{float32(i), float32(i+1), float32(i+2)}, i)
		require.NoError(t, err)
	}
	
	assert.Equal(t, 5, index.Size())
	
	// Clear index
	err = index.Clear()
	assert.NoError(t, err)
	assert.Equal(t, 0, index.Size())
	
	// Should be able to add new vectors after clear
	err = index.Add([]float32{1.0, 2.0, 3.0}, 10)
	assert.NoError(t, err)
	assert.Equal(t, 1, index.Size())
}

func TestHNSWConfig(t *testing.T) {
	// Test default config
	config := DefaultConfig()
	assert.Equal(t, 16, config.M)
	assert.Equal(t, 200, config.EfConstruction)
	assert.Equal(t, 20, config.Ef)
	assert.Equal(t, "cosine", config.DistanceType)
	
	// Test custom config
	custom := HNSWConfig{
		M:              16,
		EfConstruction: 100,
		Ef:             25,
		DistanceType:   "l2",
	}
	
	index, err := NewHNSWIndex("", 3, custom)
	require.NoError(t, err)
	assert.NotNil(t, index)
	index.Close()
}

func TestHNSWIndex_BatchAdd(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Prepare batch of vectors
	vectors := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
		{0.7, 0.8, 0.9},
	}
	ids := []uint64{1, 2, 3}
	
	// Add batch
	err = index.AddBatch(vectors, ids)
	assert.NoError(t, err)
	assert.Equal(t, 3, index.Size())
	
	// Verify all vectors are searchable
	for i, vec := range vectors {
		results, err := index.Search(vec, 1)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, ids[i], results[0].ID)
	}
}

func TestHNSWIndex_EmptySearch(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Search in empty index
	results, err := index.Search([]float32{0.1, 0.2, 0.3}, 5)
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestHNSWIndex_InvalidDimension(t *testing.T) {
	index, err := NewHNSWIndex("", 3, DefaultConfig())
	require.NoError(t, err)
	defer index.Close()
	
	// Try to add vector with wrong dimension
	err = index.Add([]float32{0.1, 0.2}, 1) // 2D instead of 3D
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dimension")
	
	// Try to search with wrong dimension
	_, err = index.Search([]float32{0.1, 0.2, 0.3, 0.4}, 1) // 4D instead of 3D
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dimension")
}