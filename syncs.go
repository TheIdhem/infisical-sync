package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func slugify(s string) string {
	return strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(strings.ToLower(s))
}

func (c *InfisicalClient) createSecretsManagerSync(projectID, envName, category, secretPath, connID, region, credentialPrefix string) error {
	syncName := "sm-" + envName + "-" + slugify(syncNameFromPath(secretPath, category))
	if len(syncName) > 256 {
		syncName = syncName[:256]
	}
	awsSecretName := awsDestinationName(secretPath, credentialPrefix)

	body := map[string]interface{}{
		"name":         syncName,
		"projectId":    projectID,
		"connectionId": connID,
		"environment":  envName,
		"secretPath":   secretPath,
		"syncOptions": map[string]interface{}{
			"initialSyncBehavior": "overwrite-destination",
		},
		"destinationConfig": map[string]interface{}{
			"region":          region,
			"mappingBehavior": "many-to-one",
			"secretName":      awsSecretName,
		},
	}
	resp, respBody, err := c.do("POST", "/api/v1/secret-syncs/aws-secrets-manager", body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict {
		log.Printf("OK  SM sync   env=%-10s  %s → aws:%s", envName, secretPath, awsSecretName)
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
}

func (c *InfisicalClient) createParameterStoreSync(projectID, envName, category, secretPath, connID, region, credentialPrefix string) error {
	syncName := "ps-" + envName + "-" + slugify(syncNameFromPath(secretPath, category))
	if len(syncName) > 256 {
		syncName = syncName[:256]
	}
	psPath := "/" + awsDestinationName(secretPath, credentialPrefix) + "/"

	body := map[string]interface{}{
		"name":         syncName,
		"projectId":    projectID,
		"connectionId": connID,
		"environment":  envName,
		"secretPath":   secretPath,
		"syncOptions": map[string]interface{}{
			"initialSyncBehavior": "overwrite-destination",
		},
		"destinationConfig": map[string]interface{}{
			"region": region,
			"path":   psPath,
		},
	}
	resp, respBody, err := c.do("POST", "/api/v1/secret-syncs/aws-parameter-store", body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict {
		log.Printf("OK  PS sync   env=%-10s  %s → aws:%s", envName, secretPath, psPath)
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
}

func createSync(client *InfisicalClient, projectID, envName, category string, key SyncKey, awsConnID, awsRegion, credentialPrefix string) error {
	switch key.RecommendedStore {
	case "Secrets Manager":
		return client.createSecretsManagerSync(projectID, envName, category, key.SecretPath, awsConnID, awsRegion, credentialPrefix)
	case "Parameter Store":
		return client.createParameterStoreSync(projectID, envName, category, key.SecretPath, awsConnID, awsRegion, credentialPrefix)
	default:
		return fmt.Errorf("unknown store %q", key.RecommendedStore)
	}
}

func (c *InfisicalClient) listSyncs(syncType, projectID, envName string) ([]syncEntry, error) {
	path := "/api/v1/secret-syncs/" + syncType + "?projectId=" + url.QueryEscape(projectID)
	resp, body, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		SecretSyncs []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Folder struct {
				Path string `json:"path"`
			} `json:"folder"`
			Environment struct {
				Slug string `json:"slug"`
			} `json:"environment"`
		} `json:"secretSyncs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var out []syncEntry
	for _, s := range result.SecretSyncs {
		if envName == "" || s.Environment.Slug == envName {
			out = append(out, syncEntry{ID: s.ID, Name: s.Name, SecretPath: s.Folder.Path, EnvName: s.Environment.Slug})
		}
	}
	return out, nil
}

func (c *InfisicalClient) listSyncsByCategory(syncType, projectID, category string) ([]syncEntry, error) {
	var out []syncEntry
	for _, envName := range categoryEnvs[category] {
		syncs, err := c.listSyncs(syncType, projectID, envName)
		if err != nil {
			return nil, fmt.Errorf("list syncs for %s: %w", envName, err)
		}
		out = append(out, syncs...)
	}
	return out, nil
}

func syncExistsForPath(syncs []syncEntry, secretPath string) bool {
	for _, s := range syncs {
		if strings.TrimLeft(s.SecretPath, "/") == secretPath {
			return true
		}
	}
	return false
}

func syncExistsForCategoryPath(syncs []syncEntry, category, folder string) (bool, error) {
	envs := categoryEnvs[category]
	withSync := map[string]bool{}
	for _, s := range syncs {
		if strings.TrimLeft(s.SecretPath, "/") == folder {
			withSync[s.EnvName] = true
		}
	}

	var have, missing []string
	for _, env := range envs {
		if withSync[env] {
			have = append(have, env)
		} else {
			missing = append(missing, env)
		}
	}

	if len(have) > 0 && len(missing) > 0 {
		return false, fmt.Errorf(
			"inconsistent sync state for %s in %s: %v have it, %v do not — fix manually before proceeding",
			folder, category, have, missing,
		)
	}
	return len(have) == len(envs) && len(envs) > 0, nil
}

func (c *InfisicalClient) deleteSync(syncType, syncID string) error {
	path := "/api/v1/secret-syncs/" + syncType + "/" + url.PathEscape(syncID) + "?removeSecrets=true"
	resp, body, err := c.do("DELETE", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
}
