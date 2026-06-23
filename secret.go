package main

import "log"

var storeToAPIType = map[string]string{
	"Secrets Manager": "aws-secrets-manager",
	"Parameter Store": "aws-parameter-store",
}

func runSingleSecret(
	client *InfisicalClient,
	projectID, folder, secretName, secretValue, storeType,
	awsRoleName, credentialPrefix, targetCategory string,
) {
	for category, envs := range categoryEnvs {
		if targetCategory != "" && targetCategory != category {
			continue
		}

		requestedApiType := storeToAPIType[storeType]
		existingSync := false
		for _, apiType := range []string{"aws-secrets-manager", "aws-parameter-store"} {
			availableSyncs, err := client.listSyncsByCategory(apiType, projectID, category)
			if err != nil {
				log.Printf("WARN list %s syncs for %s: %v", apiType, category, err)
				continue
			}
			exists, err := syncExistsForCategoryPath(availableSyncs, category, folder)
			if err != nil {
				log.Fatalf("ERROR %v", err)
			}
			if exists && apiType != requestedApiType {
				log.Fatalf("ERROR secret %s/%s already synced as %s in %s — remove the existing sync first before adding a %s sync", folder, secretName, apiType, category, requestedApiType)
			}
			if exists {
				existingSync = true
			}
		}

		if err := client.ensureEnvironment(projectID, category); err != nil {
			log.Printf("WARN ensure category env %q: %v — skipping", category, err)
			continue
		}
		if err := client.ensureFolderPath(projectID, category, folder); err != nil {
			log.Printf("WARN folder %s in %s: %v", folder, category, err)
		}
		exists, err := client.secretExists(projectID, category, folder, secretName)
		if err != nil {
			log.Printf("WARN check secret %s/%s in %s: %v", folder, secretName, category, err)
		} else if exists {
			log.Printf("WARN secret %s/%s already exists in %s — skipping", folder, secretName, category)
		} else if err := client.createSecret(projectID, category, folder, secretName, secretValue, storeType); err != nil {
			log.Fatalf("ERROR create secret %s/%s in %s: %v", folder, secretName, category, err)
		} else {
			log.Printf("OK  secret  env=%-12s  %s/%s", category, folder, secretName)
		}

		for _, envName := range envs {
			if err := client.ensureEnvironment(projectID, envName); err != nil {
				log.Printf("WARN ensure env %q: %v — skipping", envName, err)
				continue
			}

			if err := client.ensureFolderPath(projectID, envName, folder); err != nil {
				log.Printf("WARN folder %s in %s: %v", folder, envName, err)
			}

			connName := "theidhem-aws-" + envName
			region := regionForEnv(envName)
			awsRoleArn, err := roleARNForEnv(envName, awsRoleName)
			if err != nil {
				log.Printf("WARN role ARN for %s: %v — skipping AWS sync", envName, err)
				goto secretImport
			}

			{
				awsConnID, err := client.ensureAWSConnection(connName, awsRoleArn, projectID)
				if err != nil {
					log.Printf("WARN AWS connection for %s: %v — skipping sync", envName, err)
					goto secretImport
				}

				if existingSync {
					log.Printf("sync already exists for %s in %s [%s] — skipping", folder, envName, storeType)
				} else {
					key := SyncKey{SecretPath: folder, RecommendedStore: storeType}
					if err := createSync(client, projectID, envName, category, key, awsConnID, region, credentialPrefix); err != nil {
						log.Fatalf("ERROR sync %s [%s] for %s: %v", folder, storeType, envName, err)
					}
				}
			}

		secretImport:
			folderPath := "/" + folder
			if err := client.ensureSecretImport(projectID, envName, folderPath, category, folderPath); err != nil {
				log.Printf("WARN secret import %s:%s ← %s: %v", envName, folderPath, category, err)
			}
		}
	}
}
