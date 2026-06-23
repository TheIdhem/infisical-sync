package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var arrayVarRe = regexp.MustCompile(`^([a-zA-Z0-9_]+)\[`)
var complexVarRe = regexp.MustCompile(`^,`)

func baseVariableName(name string) string {
	if m := arrayVarRe.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return name
}

func splitAWSPath(p string) (folder, name string) {
	p = strings.TrimSpace(p)
	if idx := strings.Index(p, " "); idx != -1 {
		p = p[:idx]
	}
	p = strings.TrimRight(p, "/")
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/", p
	}
	return p[:idx], p[idx+1:]
}

func awsDestinationName(secretPath, prefix string) string {
	p := strings.TrimLeft(secretPath, "/")
	if prefix == "" {
		return p
	}
	return prefix + "/" + p
}

func syncNameFromPath(p, category string) string {
	p = strings.TrimLeft(p, "/")
	for _, prefix := range []string{category + "/", "shared/"} {
		if strings.HasPrefix(p, prefix) {
			return p[len(prefix):]
		}
	}
	return p
}

func extractFromFile(groupVarsDir, sourceFile, variableName string) (string, error) {
	path := filepath.Join(groupVarsDir, sourceFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	var yamlDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &yamlDoc); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}

	parts := strings.Split(variableName, ".")
	var node interface{} = yamlDoc
	for _, part := range parts {
		m, ok := node.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("key path %q: expected map at %q", variableName, part)
		}
		node, ok = m[part]
		if !ok {
			return "", fmt.Errorf("key %q not found in %s", part, sourceFile)
		}
	}

	switch v := node.(type) {
	case string:
		return v, nil
	case int, int64, float64, bool:
		return fmt.Sprintf("%v", v), nil
	default:
		return "", fmt.Errorf("value for %q is a %T (not scalar) — skip", variableName, node)
	}
}

func extractValue(groupVarsDir, sourceFile, variableName, category string) (string, error) {
	val, err := extractFromFile(groupVarsDir, sourceFile, variableName)
	if err != nil && sourceFile == "all.yml" {
		fallback := category + ".yml"
		val2, err2 := extractFromFile(groupVarsDir, fallback, variableName)
		if err2 == nil {
			log.Printf("NOTE  %s not in all.yml, found in %s", variableName, fallback)
			return val2, nil
		}
		return "", fmt.Errorf("not found in all.yml or %s", fallback)
	}
	return val, err
}

func processRow(client *InfisicalClient, row CSVRow, envName, category, projectID, groupVarsDir string, syncs map[SyncKey]bool, dryRun bool) error {
	if complexVarRe.MatchString(row.VariableName) {
		log.Printf("SKIP complex: %s", row.VariableName)
		return nil
	}
	if !strings.HasSuffix(row.SourceFile, ".yml") {
		log.Printf("SKIP non-YAML source: %s (%s)", row.VariableName, row.SourceFile)
		return nil
	}

	folder, secretName := splitAWSPath(row.SuggestedAWSKeyPath)
	if secretName == "" {
		return fmt.Errorf("cannot derive secret name from path %q", row.SuggestedAWSKeyPath)
	}
	folder = syncNameFromPath(folder, category)

	lookupKey := baseVariableName(row.VariableName)
	value, err := extractValue(groupVarsDir, row.SourceFile, lookupKey, category)

	if dryRun {
		valueNote := "<found>"
		if err != nil {
			valueNote = "<not found: " + err.Error() + ">"
		} else if value == "" {
			valueNote = "<empty>"
		}
		fmt.Printf("[DRY] env=%-10s  [%-17s]  folder=%-35s  name=%-30s  value=%s\n",
			envName, row.RecommendedStore, folder, secretName, valueNote)
		if err == nil && value != "" {
			syncs[SyncKey{SecretPath: folder, RecommendedStore: row.RecommendedStore}] = true
		}
		return nil
	}

	if err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("empty value for %s in %s", row.VariableName, row.SourceFile)
	}

	if err := client.createSecret(projectID, envName, folder, secretName, value, row.RecommendedStore); err != nil {
		return fmt.Errorf("create secret %s/%s: %w", folder, secretName, err)
	}
	log.Printf("OK  secret  env=%-10s  [%-17s]  %s/%s", envName, row.RecommendedStore, folder, secretName)
	syncs[SyncKey{SecretPath: folder, RecommendedStore: row.RecommendedStore}] = true
	return nil
}
