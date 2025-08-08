package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/andrejsstepanovs/codesearch/models"
	fastshot "github.com/opus-domini/fast-shot"
)

type Litellm struct {
}

func client(name string) fastshot.ClientHttpMethods {
	clients := map[string][]string{
		"litellm": {"http://localhost:4000", "sk-1234"},
		"ollama":  {"http://localhost:11434", ""},
	}

	c := fastshot.NewClient(clients[name][0])
	if clients[name][1] != "" {
		c.Auth().BearerToken(clients[name][1])
	}

	return c.Config().SetTimeout(time.Minute).
		Config().SetFollowRedirects(true).
		Header().Add("Content-Type", "application/json").
		Build()
}

// Embeddings retrieves text embeddings from the LiteLLM service.
func Embeddings(ctx context.Context, clientName, model, inputText string) (models.EmbeddingResponse, error) {
	if inputText == "" {
		return models.EmbeddingResponse{}, fmt.Errorf("inputText cannot be empty")
	}

	req := models.EmbeddingRequest{
		Model: model,
		Input: inputText,
	}

	var path string
	switch clientName {
	case "litellm":
		path = "/v1/embeddings"
	case "ollama":
		path = "/api/embed"
	default:
		return models.EmbeddingResponse{}, fmt.Errorf("unsupported client: %s", clientName)
	}

	resp, err := client(clientName).
		POST(path).
		Context().Set(ctx).
		Header().Add("Accept", "application/json").
		Retry().SetExponentialBackoff(time.Second*30, 4, 2.0).
		Body().AsJSON(req).
		Send()

	if err != nil {
		return models.EmbeddingResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body().Close()

	var res models.EmbeddingResponse
	err = parseHTTPResponse(*resp, &res)
	if err != nil {
		return models.EmbeddingResponse{}, err
	}

	return res, nil
}

func parseHTTPResponse[T any](resp fastshot.Response, result *T) error {
	if resp.Status().IsError() {
		msg, err := resp.Body().AsString()
		if err != nil {
			return fmt.Errorf("failed to read error response: %w", err)
		}
		return errors.New(msg)
	}

	err := resp.Body().AsJSON(result)
	if err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
