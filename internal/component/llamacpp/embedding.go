package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
)

var _ component.EmbeddingEngine = (*Component)(nil)

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (c *Component) Embed(ctx context.Context, req component.EmbeddingRequest) (component.EmbeddingResponse, error) {
	modelName := strings.TrimSpace(req.Model)
	runtime, model, err := c.runtimeForModel(modelName)
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	if cleanModelMode(model.Mode) != "embedding" {
		return component.EmbeddingResponse{}, fmt.Errorf("llama.cpp model %s is not configured for embeddings", model.Name)
	}
	session, err := c.BeginInferenceSession(ctx, component.InferenceSessionOptions{Model: model.Name})
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	defer func() { _ = session.Close() }()

	inputs := cleanEmbeddingInputs(req.Inputs)
	if len(inputs) == 0 {
		return component.EmbeddingResponse{}, fmt.Errorf("missing embedding input")
	}
	release, err := c.acquireInference(ctx, model.Name)
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	defer release()
	texts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		texts = append(texts, input.Text)
	}
	body, err := json.Marshal(embeddingRequest{Model: model.Name, Input: texts})
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, runtime.BaseURL()+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return component.EmbeddingResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return component.EmbeddingResponse{}, fmt.Errorf("llamacpp embedding status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	var decoded embeddingResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return component.EmbeddingResponse{}, err
	}
	out := make([]component.Embedding, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(inputs) {
			continue
		}
		vector := append([]float32(nil), item.Embedding...)
		normalized := false
		if model.Normalize {
			normalizeVector(vector)
			normalized = true
		}
		out = append(out, component.Embedding{
			ID:         inputs[item.Index].ID,
			Vector:     vector,
			Dim:        len(vector),
			Model:      model.Name,
			Normalized: normalized,
		})
	}
	return component.EmbeddingResponse{Embeddings: out}, nil
}

func cleanEmbeddingInputs(inputs []component.EmbeddingInput) []component.EmbeddingInput {
	out := make([]component.EmbeddingInput, 0, len(inputs))
	for _, input := range inputs {
		input.ID = strings.TrimSpace(input.ID)
		input.Text = strings.TrimSpace(input.Text)
		if input.Text == "" {
			continue
		}
		out = append(out, input)
	}
	return out
}

func normalizeVector(vector []float32) {
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if sum <= 0 {
		return
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range vector {
		vector[i] *= scale
	}
}
