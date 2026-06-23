package main

import (
	"flag"
	"log"
	"os"
	"strings"
)

func secretMain() {
	fs := flag.NewFlagSet("secret", flag.ExitOnError)

	clientID := fs.String("client-id", os.Getenv("INFISICAL_CLIENT_ID"), "Universal Auth client ID")
	clientSecret := fs.String("client-secret", os.Getenv("INFISICAL_CLIENT_SECRET"), "Universal Auth client secret")
	projectName := fs.String("project-name", "Engine-Mehdi", "Infisical project name")
	awsRoleName := fs.String("aws-role-name", os.Getenv("INFISICAL_AWS_ROLE_NAME"), "AWS IAM role name")
	credentialPrefix := fs.String("credential-prefix", "", "Prefix for AWS secret/parameter names")
	baseURL := fs.String("base-url", defaultBaseURL, "Infisical base URL")

	secretPath := fs.String("secret-path", "", "Folder path (e.g. kafka)")
	secretName := fs.String("secret-name", "", "Secret key name (e.g. username)")
	secretValue := fs.String("secret-value", "", "Secret value")
	storeType := fs.String("store-type", "Secrets Manager", "AWS store: 'Secrets Manager' or 'Parameter Store'")
	category := fs.String("category", "", "Limit to one category: development, sandbox, or production (default: all)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	if *secretPath == "" || *secretName == "" || *secretValue == "" {
		log.Fatal("--secret-path, --secret-name, and --secret-value are all required")
	}
	if *clientID == "" || *clientSecret == "" {
		log.Fatal("--client-id and --client-secret are required (or INFISICAL_CLIENT_ID / INFISICAL_CLIENT_SECRET)")
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

	cleanPath := strings.Trim(*secretPath, "/")
	runSingleSecret(client, projectID, cleanPath, *secretName, *secretValue, *storeType, *awsRoleName, *credentialPrefix, *category)
	log.Println("done")
}
