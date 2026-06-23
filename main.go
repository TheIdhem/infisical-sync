package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "secret" {
		secretMain()
		return
	}
	envName := flag.String("env", "", "Infisical environment slug or category (e.g. rocks, development, io)")
	clientID := flag.String("client-id", os.Getenv("INFISICAL_CLIENT_ID"), "Universal Auth client ID")
	clientSecret := flag.String("client-secret", os.Getenv("INFISICAL_CLIENT_SECRET"), "Universal Auth client secret")
	projectName := flag.String("project-name", "Engine-Mehdi", "Infisical project name")
	awsRoleName := flag.String("aws-role-name", os.Getenv("INFISICAL_AWS_ROLE_NAME"), "AWS IAM role name (ARN constructed automatically per env)")
	awsConnName := flag.String("aws-connection-name", "", "AWS app connection name override (single-env mode only; defaults to theidhem-aws-<env>)")
	awsRegion := flag.String("aws-region", "", "AWS region override (single-env mode only; auto-detected otherwise)")
	credentialPrefix := flag.String("credential-prefix", "", "Prefix prepended to all AWS secret/parameter names (e.g. theidhem)")
	cleanup := flag.Bool("cleanup", false, "Delete all secrets, syncs, and connections for this env/category")
	csvPath := flag.String("csv", "bin/infisical-sync/seeds/secrets-inventory.csv", "Path to secrets inventory CSV")
	groupVarsDir := flag.String("group-vars-dir", "infra/ansible/group_vars", "Path to Ansible group_vars directory")
	baseURL := flag.String("base-url", defaultBaseURL, "Infisical base URL")
	dryRun := flag.Bool("dry-run", false, "Print actions without calling the API")
	flag.Parse()

	if *envName == "" {
		log.Fatal("--env is required")
	}

	category, err := categoryForEnv(*envName)
	if err != nil {
		log.Fatal(err)
	}

	if err := validateFlags(*clientID, *clientSecret, *dryRun); err != nil {
		log.Fatal(err)
	}

	if !*dryRun {
		requireConsent(*envName, *cleanup)
	}

	rows, err := parseCSV(*csvPath)
	if err != nil {
		log.Fatalf("parse CSV: %v", err)
	}
	filtered := filterRows(rows, category)
	log.Printf("category=%s  rows=%d", category, len(filtered))

	if *dryRun {
		runDry(*envName, category, filtered, *groupVarsDir)
		return
	}

	client, err := newClient(*baseURL, *clientID, *clientSecret)
	if err != nil {
		log.Fatalf("authentication failed: %v", err)
	}
	log.Println("authenticated with Infisical")

	projectID, err := client.findProjectID(*projectName)
	if err != nil {
		log.Fatalf("find project %q: %v", *projectName, err)
	}
	log.Printf("project %q → %s", *projectName, projectID)

	if isCategoryEnv(*envName) {
		runCategory(client, projectID, category, *awsRoleName, *credentialPrefix, *groupVarsDir, filtered, *cleanup)
	} else {
		connName := *awsConnName
		if connName == "" {
			connName = "theidhem-aws-" + *envName
		}
		region := *awsRegion
		if region == "" {
			region = regionForEnv(*envName)
		}
		runSingleEnv(client, projectID, *envName, category, connName, *awsRoleName, region, *credentialPrefix, *groupVarsDir, filtered, "", *cleanup)
	}

	log.Println("done")
}

func runCategory(client *InfisicalClient, projectID, category, awsRoleName, credentialPrefix, groupVarsDir string, rows []CSVRow, doCleanup bool) {
	if doCleanup {
		for _, env := range categoryEnvs[category] {
			connName := "theidhem-aws-" + env
			region := regionForEnv(env)
			runSingleEnv(client, projectID, env, category, connName, awsRoleName, region, credentialPrefix, groupVarsDir, rows, "", true)
		}
		runCleanup(client, projectID, category, category, "", rows, groupVarsDir)
		return
	}

	if err := client.ensureEnvironment(projectID, category); err != nil {
		log.Fatalf("ensure category environment %q: %v", category, err)
	}
	baseSyncs := make(map[SyncKey]bool)
	for _, row := range rows {
		if err := processRow(client, row, category, category, projectID, groupVarsDir, baseSyncs, false); err != nil {
			log.Printf("WARN base [%s] %s: %v", category, row.VariableName, err)
		}
	}
	log.Printf("base secrets written to %q environment", category)

	for _, env := range categoryEnvs[category] {
		connName := "theidhem-aws-" + env
		region := regionForEnv(env)
		runSingleEnv(client, projectID, env, category, connName, awsRoleName, region, credentialPrefix, groupVarsDir, rows, category, false)
	}
}

func runSingleEnv(client *InfisicalClient, projectID, envName, category, connName, awsRoleName, awsRegion, credentialPrefix, groupVarsDir string, rows []CSVRow, importFrom string, doCleanup bool) {
	log.Printf("--- env=%s ---", envName)

	if err := client.ensureEnvironment(projectID, envName); err != nil {
		log.Printf("WARN ensure environment %q: %v — skipping", envName, err)
		return
	}

	awsRoleArn, err := roleARNForEnv(envName, awsRoleName)
	if err != nil {
		log.Printf("WARN %s: %v — skipping", envName, err)
		return
	}

	awsConnID, err := client.ensureAWSConnection(connName, awsRoleArn, projectID)
	if err != nil {
		log.Printf("WARN ensure AWS connection for %s: %v — skipping", envName, err)
		return
	}
	log.Printf("AWS connection %q → %s", connName, awsConnID)

	if doCleanup {
		for _, syncType := range []string{"aws-secrets-manager", "aws-parameter-store"} {
			syncs, err := client.listSyncs(syncType, projectID, envName)
			if err != nil {
				log.Fatalf("list %s syncs for %s: %v", syncType, envName, err)
			}
			for _, s := range syncs {
				if err := client.deleteSync(syncType, s.ID); err != nil {
					log.Fatalf("delete %s sync %s: %v", syncType, s.Name, err)
				}
				log.Printf("deleted %s sync %s", syncType, s.Name)
			}
		}

		foldersSeen := map[string]bool{"/": true}
		for _, row := range rows {
			folder, secretName := splitAWSPath(row.SuggestedAWSKeyPath)
			if secretName == "" {
				continue
			}
			folder = "/" + syncNameFromPath(folder, category)
			foldersSeen[folder] = true
		}
		for folder := range foldersSeen {
			if err := client.deleteAllSecretImports(projectID, envName, folder); err != nil {
				log.Printf("WARN delete secret imports %s %s: %v", envName, folder, err)
			}
		}
		runCleanup(client, projectID, envName, category, awsConnID, rows, groupVarsDir)
		return
	}

	syncsNeeded := make(map[SyncKey]bool)
	if importFrom != "" {

		foldersSeen := map[string]bool{}
		for _, row := range rows {
			if err := processRow(nil, row, envName, category, "", groupVarsDir, syncsNeeded, true); err != nil {
				log.Printf("WARN row %s (%s): %v", row.ID, row.VariableName, err)
				continue
			}
			folder, secretName := splitAWSPath(row.SuggestedAWSKeyPath)
			if secretName == "" {
				continue
			}
			folder = syncNameFromPath(folder, category)
			if !foldersSeen[folder] {
				foldersSeen[folder] = true
				if err := client.ensureFolderPath(projectID, envName, folder); err != nil {
					log.Printf("WARN ensure folder %s in %s: %v", folder, envName, err)
				}
				folderPath := "/" + folder
				if err := client.ensureSecretImport(projectID, envName, folderPath, importFrom, folderPath); err != nil {
					log.Printf("WARN secret import %s:%s ← %s: %v", envName, folderPath, importFrom, err)
				}
			}
		}
	} else {
		for _, row := range rows {
			if err := processRow(client, row, envName, category, projectID, groupVarsDir, syncsNeeded, false); err != nil {
				log.Printf("WARN row %s (%s): %v", row.ID, row.VariableName, err)
			}
		}
	}

	for key := range syncsNeeded {
		if err := createSync(client, projectID, envName, category, key, awsConnID, awsRegion, credentialPrefix); err != nil {
			log.Printf("WARN sync %s [%s]: %v", key.SecretPath, key.RecommendedStore, err)
		}
	}
}

func runDry(envName, category string, rows []CSVRow, groupVarsDir string) {
	targets := []string{category}
	if isCategoryEnv(envName) {
		targets = append(targets, categoryEnvs[category]...)
	} else {
		targets = []string{envName}
	}

	for _, target := range targets {
		syncsNeeded := make(map[SyncKey]bool)
		fmt.Printf("\n=== dry run: env=%s ===\n", target)
		for _, row := range rows {
			if err := processRow(nil, row, target, category, "", groupVarsDir, syncsNeeded, true); err != nil {
				log.Printf("WARN dry %s (%s): %v", row.ID, row.VariableName, err)
			}
		}
		fmt.Printf("  syncs that would be created:\n")
		for k := range syncsNeeded {
			fmt.Printf("    [%-17s]  %s\n", k.RecommendedStore, k.SecretPath)
		}
	}
}

var sensitiveEnvs = map[string]bool{
	"sandbox":    true,
	"production": true,
	"how":        true,
	"io":         true,
}

func requireConsent(envName string, doCleanup bool) {
	if !sensitiveEnvs[envName] {
		return
	}
	action := "apply secrets and syncs"
	if doCleanup {
		action = "DESTROY all secrets, syncs, and connections"
	}
	fmt.Printf("\n⚠  WARNING: you are about to %s on %q\n", action, envName)
	fmt.Printf("   Type %q to confirm: ", envName)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if strings.TrimSpace(scanner.Text()) != envName {
		log.Fatal("consent not confirmed — aborting")
	}
}

func validateFlags(clientID, clientSecret string, dryRun bool) error {
	if !dryRun && (clientID == "" || clientSecret == "") {
		return fmt.Errorf("--client-id and --client-secret are required (or INFISICAL_CLIENT_ID / INFISICAL_CLIENT_SECRET)")
	}
	return nil
}

func hasYMLSuffix(s string) bool {
	return len(s) > 4 && s[len(s)-4:] == ".yml"
}
