package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
)

func (c *InfisicalClient) secretExists(projectID, environment, secretPath, secretName string) (bool, error) {
	q := fmt.Sprintf("/api/v4/secrets/%s?projectId=%s&environment=%s&secretPath=%s",
		url.PathEscape(secretName),
		url.QueryEscape(projectID),
		url.QueryEscape(environment),
		url.QueryEscape(secretPath),
	)
	resp, body, err := c.do("GET", q, nil)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("check secret HTTP %d: %s", resp.StatusCode, body)
}

func (c *InfisicalClient) createSecret(projectID, environment, secretPath, secretName, secretValue, storeType string) error {
	body := map[string]interface{}{
		"projectId":   projectID,
		"environment": environment,
		"secretPath":  secretPath,
		"secretValue": secretValue,
		"secretMetadata": []map[string]string{
			{"key": "store-type", "value": storeType},
		},
	}
	resp, respBody, err := c.do("POST", "/api/v4/secrets/"+url.PathEscape(secretName), body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}
	if resp.StatusCode == http.StatusConflict {
		log.Printf("secret already exists: %s/%s — updating", secretPath, secretName)
		return c.updateSecret(projectID, environment, secretPath, secretName, secretValue, storeType)
	}
	// 404 = parent folder missing — create it and retry once
	if resp.StatusCode == http.StatusNotFound {
		if err := c.ensureFolderPath(projectID, environment, secretPath); err != nil {
			return fmt.Errorf("ensure folder %s: %w", secretPath, err)
		}
		resp, respBody, err = c.do("POST", "/api/v4/secrets/"+url.PathEscape(secretName), body)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			return nil
		}
		if resp.StatusCode == http.StatusConflict {
			return c.updateSecret(projectID, environment, secretPath, secretName, secretValue, storeType)
		}
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
}

func (c *InfisicalClient) updateSecret(projectID, environment, secretPath, secretName, secretValue, storeType string) error {
	body := map[string]interface{}{
		"projectId":   projectID,
		"environment": environment,
		"secretPath":  secretPath,
		"secretValue": secretValue,
		"secretMetadata": []map[string]string{
			{"key": "store-type", "value": storeType},
		},
	}
	resp, respBody, err := c.do("PATCH", "/api/v4/secrets/"+url.PathEscape(secretName), body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("update HTTP %d: %s", resp.StatusCode, respBody)
}

func (c *InfisicalClient) deleteSecret(projectID, environment, secretPath, secretName string) error {
	path := "/api/v4/secrets/" + url.PathEscape(secretName)
	payload := map[string]interface{}{
		"projectId":   projectID,
		"environment": environment,
		"secretPath":  secretPath,
	}
	resp, body, err := c.do("DELETE", path, payload)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
}
