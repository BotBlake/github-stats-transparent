package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func (c *Client) QueryGraphQL(ctx context.Context, query string) (map[string]any, error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	payload, _ := json.Marshal(map[string]string{
		"query": query,
	})

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.github.com/graphql",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}
