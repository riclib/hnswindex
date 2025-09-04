package embedder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockEmbedder for testing
type MockEmbedder struct {
	mock.Mock
}

func (m *MockEmbedder) GenerateEmbedding(text string) ([]float32, error) {
	args := m.Called(text)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockEmbedder) GenerateEmbeddings(texts []string) ([][]float32, error) {
	args := m.Called(texts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockEmbedder) Dimension() int {
	args := m.Called()
	return args.Int(0)
}

func TestEmbedderInterface(t *testing.T) {
	mockEmb := new(MockEmbedder)
	
	// Test single embedding
	expectedEmbedding := []float32{0.1, 0.2, 0.3}
	mockEmb.On("GenerateEmbedding", "test text").Return(expectedEmbedding, nil)
	
	embedding, err := mockEmb.GenerateEmbedding("test text")
	require.NoError(t, err)
	assert.Equal(t, expectedEmbedding, embedding)
	
	// Test batch embeddings
	texts := []string{"text1", "text2"}
	expectedEmbeddings := [][]float32{{0.1, 0.2}, {0.3, 0.4}}
	mockEmb.On("GenerateEmbeddings", texts).Return(expectedEmbeddings, nil)
	
	embeddings, err := mockEmb.GenerateEmbeddings(texts)
	require.NoError(t, err)
	assert.Equal(t, expectedEmbeddings, embeddings)
	
	// Test dimension
	mockEmb.On("Dimension").Return(768)
	assert.Equal(t, 768, mockEmb.Dimension())
	
	mockEmb.AssertExpectations(t)
}

func TestNewOllamaEmbedder(t *testing.T) {
	// Test with default values
	emb, err := NewOllamaEmbedder("http://localhost:11434", "nomic-embed-text")
	require.NoError(t, err)
	assert.NotNil(t, emb)
	
	// For nomic-embed-text, dimension should be 768
	assert.Equal(t, 768, emb.Dimension())
}

func TestOllamaEmbedder_InvalidURL(t *testing.T) {
	_, err := NewOllamaEmbedder("", "nomic-embed-text")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL cannot be empty")
}

func TestOllamaEmbedder_InvalidModel(t *testing.T) {
	_, err := NewOllamaEmbedder("http://localhost:11434", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model cannot be empty")
}

func TestBatchProcessing(t *testing.T) {
	// This will test the actual batch processing with worker pool
	// when we have the implementation
	t.Skip("Integration test - requires implementation")
}