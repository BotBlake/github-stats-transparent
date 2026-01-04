package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (c *Client) QueryREST(ctx context.Context, path string) (map[string]any, error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		"https://api.github.com/"+strings.TrimPrefix(path, "/"),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 202 {
		return nil, errors.New("202 accepted")
	}

	var result map[string]any
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}
