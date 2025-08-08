package models

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type Embedding []float64

type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding Embedding `json:"embedding"`
	Index     int       `json:"index"`
}

type EmbeddingResponse struct {
	Object     string          `json:"object"`
	Embeddings []Embedding     `json:"embeddings"` // ollama response
	Data       []EmbeddingData `json:"data"`       // litellm response
	Model      string          `json:"model"`
	Usage      EmbeddingUsage  `json:"usage"`
}

func (er EmbeddingResponse) GetEmbeddings() *Embedding {
	if len(er.Embeddings) > 0 {
		return &er.Embeddings[0]
	}
	if len(er.Data) > 0 {
		return &er.Data[0].Embedding
	}
	return nil
}

func (e *Embedding) Float32() []float32 {
	float32s := make([]float32, len(*e))
	for i, v := range *e {
		float32s[i] = float32(v)
	}
	return float32s
}

type EmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}
