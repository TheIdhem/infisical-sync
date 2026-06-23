package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type InfisicalClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func newClient(baseURL, clientID, clientSecret string) (*InfisicalClient, error) {
	form := url.Values{
		"clientId":     {clientID},
		"clientSecret": {clientSecret},
	}
	resp, err := http.PostForm(baseURL+"/api/v1/auth/universal-auth/login", form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("no accessToken in response")
	}
	return &InfisicalClient{baseURL: baseURL, token: result.AccessToken, httpClient: &http.Client{}}, nil
}

func (c *InfisicalClient) do(method, path string, body interface{}) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody, nil
}

func (c *InfisicalClient) findProjectID(name string) (string, error) {
	resp, body, err := c.do("GET", "/api/v1/projects", nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list projects HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse projects response: %w", err)
	}
	for _, p := range result.Projects {
		if p.Name == name {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("project %q not found", name)
}
