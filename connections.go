package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

func (c *InfisicalClient) ensureAWSConnection(name, roleArn, projectID string) (string, error) {
	resp, body, err := c.do("GET", "/api/v1/app-connections/aws", nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list connections HTTP %d: %s", resp.StatusCode, body)
	}

	var listResp struct {
		AppConnections []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			ProjectID string `json:"projectId"`
		} `json:"appConnections"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return "", fmt.Errorf("parse connections response: %w", err)
	}
	for _, conn := range listResp.AppConnections {
		if conn.ProjectID == projectID && conn.Name == name {
			log.Printf("found existing AWS connection %q (%s)", name, conn.ID)
			return conn.ID, nil
		}
	}

	if roleArn == "" {
		return "", fmt.Errorf("no existing AWS connection found for project and --aws-role-arn (AWS_ROLE_ARN) is not set")
	}
	log.Printf("creating AWS connection %q with assume-role", name)
	payload := map[string]interface{}{
		"name":      name,
		"method":    "assume-role",
		"projectId": projectID,
		"credentials": map[string]string{
			"roleArn": roleArn,
		},
	}
	resp, body, err = c.do("POST", "/api/v1/app-connections/aws", payload)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create connection HTTP %d: %s", resp.StatusCode, body)
	}

	var createResp struct {
		AppConnection struct {
			ID string `json:"id"`
		} `json:"appConnection"`
	}
	if err := json.Unmarshal(body, &createResp); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	if createResp.AppConnection.ID == "" {
		return "", fmt.Errorf("no ID in create connection response")
	}
	log.Printf("created AWS connection %q (%s)", name, createResp.AppConnection.ID)
	return createResp.AppConnection.ID, nil
}

func (c *InfisicalClient) deleteAWSConnection(connID string) error {
	resp, body, err := c.do("DELETE", "/api/v1/app-connections/aws/"+url.PathEscape(connID), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
}
