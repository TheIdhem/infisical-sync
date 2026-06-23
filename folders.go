package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *InfisicalClient) ensureFolderPath(projectID, environment, folderPath string) error {
	segments := strings.Split(strings.Trim(folderPath, "/"), "/")
	parent := ""
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		listPath := fmt.Sprintf("/api/v2/folders?projectId=%s&environment=%s&path=%s",
			url.QueryEscape(projectID), url.QueryEscape(environment), url.QueryEscape(parent))
		resp, respBody, err := c.do("GET", listPath, nil)
		if err != nil {
			return err
		}
		var listResp struct {
			Folders []struct {
				Name string `json:"name"`
			} `json:"folders"`
		}
		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(respBody, &listResp); err != nil {
				return fmt.Errorf("parse folders response: %w", err)
			}
		} else if resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("list folders under %q: HTTP %d: %s", parent, resp.StatusCode, respBody)
		}
		exists := false
		for _, f := range listResp.Folders {
			if f.Name == seg {
				exists = true
				break
			}
		}
		if !exists {
			body := map[string]interface{}{
				"projectId":   projectID,
				"environment": environment,
				"name":        seg,
				"path":        parent,
			}
			resp, respBody, err := c.do("POST", "/api/v2/folders", body)
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				return fmt.Errorf("create folder %q under %q: HTTP %d: %s", seg, parent, resp.StatusCode, respBody)
			}
		}
		if parent == "" {
			parent = seg
		} else {
			parent = parent + "/" + seg
		}
	}
	return nil
}

func (c *InfisicalClient) deleteFolder(projectID, environment, parentPath, name string) error {
	payload := map[string]interface{}{
		"projectId":   projectID,
		"environment": environment,
		"path":        parentPath,
		"forceDelete": true,
	}
	resp, body, err := c.do("DELETE", "/api/v2/folders/"+url.PathEscape(name), payload)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
}
