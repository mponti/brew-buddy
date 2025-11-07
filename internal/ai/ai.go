package ai

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Client wraps the GenAI client.
type Client struct {
	genaiClient *genai.Client
	model       *genai.EmbeddingModel
}

// NewClient creates a connected AI client.
func NewClient(ctx context.Context) (*Client, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	c, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client: %w", err)
	}

	return &Client{
		genaiClient: c,
		model:       c.EmbeddingModel("text-embedding-004"),
	}, nil
}

// Close terminates the connection.
func (c *Client) Close() {
	if c.genaiClient != nil {
		c.genaiClient.Close()
	}
}

// EmbedString generates a vector for the given text and returns it as a byte slice (for DB storage).
// It also returns the raw []float32 if needed immediately.
func (c *Client) EmbedString(ctx context.Context, text string) ([]byte, []float32, error) {
	res, err := c.model.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, nil, err
	}
	if res.Embedding == nil {
		return nil, nil, fmt.Errorf("AI returned empty embedding")
	}

	blob, err := FloatsToBytes(res.Embedding.Values)
	if err != nil {
		return nil, nil, err
	}
	return blob, res.Embedding.Values, nil
}

// --- Vector Math Helpers ---

// CosineSimilarity calculates the similarity between two vectors (0.0 to 1.0).
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dotProduct, magA, magB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dotProduct / (float32(math.Sqrt(float64(magA))) * float32(math.Sqrt(float64(magB))))
}

// FloatsToBytes converts a []float32 slice to a []byte slice (BLOB) for SQLite.
func FloatsToBytes(floats []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, floats)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BytesToFloats converts the stored byte slice back to []float32.
func BytesToFloats(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid byte length for float32 slice")
	}
	floats := make([]float32, len(b)/4)
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &floats)
	return floats, err
}
