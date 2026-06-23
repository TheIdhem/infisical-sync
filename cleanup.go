package main

import (
	"log"
	"sort"
	"strings"
)

func runCleanup(client *InfisicalClient, projectID, envName, category, connID string, rows []CSVRow, groupVarsDir string) {
	log.Printf("--- CLEANUP  env=%s  project=%s ---", envName, projectID)

	folderSet := map[string]struct{}{}
	for _, row := range rows {
		if complexVarRe.MatchString(row.VariableName) || !hasYMLSuffix(row.SourceFile) {
			continue
		}
		folder, secretName := splitAWSPath(row.SuggestedAWSKeyPath)
		if secretName == "" {
			continue
		}
		folder = syncNameFromPath(folder, category)
		if err := client.deleteSecret(projectID, envName, folder, secretName); err != nil {
			log.Printf("WARN delete secret %s/%s: %v", folder, secretName, err)
		} else {
			log.Printf("deleted secret %s/%s", folder, secretName)
		}
		parts := strings.Split(folder, "/")
		for i := range parts {
			if ancestor := strings.Join(parts[:i+1], "/"); ancestor != "" {
				folderSet[ancestor] = struct{}{}
			}
		}
	}

	folders := make([]string, 0, len(folderSet))
	for f := range folderSet {
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool {
		return strings.Count(folders[i], "/") > strings.Count(folders[j], "/")
	})
	for _, f := range folders {
		parent := ""
		name := f
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			parent = f[:idx]
			name = f[idx+1:]
		}
		if err := client.deleteFolder(projectID, envName, parent, name); err != nil {
			log.Printf("WARN delete folder %s: %v", f, err)
		} else {
			log.Printf("deleted folder %s", f)
		}
	}

	if connID != "" {
		if err := client.deleteAWSConnection(connID); err != nil {
			log.Printf("WARN delete AWS connection %s: %v", connID, err)
		} else {
			log.Printf("deleted AWS connection %s", connID)
		}
	}

	log.Println("cleanup done")
}
