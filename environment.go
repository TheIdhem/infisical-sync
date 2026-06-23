package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func displayName(slug string) string {
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

func (c *InfisicalClient) ensureEnvironment(projectID, slug string) error {
	payload := map[string]interface{}{
		"name": displayName(slug),
		"slug": slug,
	}
	resp, body, err := c.do("POST", "/api/v1/projects/"+url.PathEscape(projectID)+"/environments", payload)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		log.Printf("created environment %q", slug)
		return nil
	}
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusBadRequest {
		log.Printf("environment %q already exists", slug)
		return nil
	}
	return fmt.Errorf("create environment HTTP %d: %s", resp.StatusCode, body)
}

func (c *InfisicalClient) ensureSecretImport(projectID, environment, path, fromEnv, fromPath string) error {
	payload := map[string]interface{}{
		"projectId":   projectID,
		"environment": environment,
		"path":        path,
		"import": map[string]string{
			"environment": fromEnv,
			"path":        fromPath,
		},
		"isReplication": true,
	}
	resp, body, err := c.do("POST", "/api/v2/secret-imports", payload)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict {
		log.Printf("secret import %s:/ ← %s:/", environment, fromEnv)
		return nil
	}
	return fmt.Errorf("create secret import HTTP %d: %s", resp.StatusCode, body)
}

func (c *InfisicalClient) deleteAllSecretImports(projectID, environment, path string) error {
	q := fmt.Sprintf("/api/v2/secret-imports?projectId=%s&environment=%s&path=%s",
		url.QueryEscape(projectID), url.QueryEscape(environment), url.QueryEscape(path))
	resp, body, err := c.do("GET", q, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list secret imports HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		SecretImports []struct {
			ID string `json:"id"`
		} `json:"secretImports"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse secret imports: %w", err)
	}

	for _, imp := range result.SecretImports {
		payload := map[string]interface{}{
			"projectId":   projectID,
			"environment": environment,
			"path":        path,
		}
		resp, body, err := c.do("DELETE", "/api/v2/secret-imports/"+url.PathEscape(imp.ID), payload)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("delete secret import %s HTTP %d: %s", imp.ID, resp.StatusCode, body)
		}
		log.Printf("deleted secret import %s from %s", imp.ID, environment)
	}
	return nil
}
