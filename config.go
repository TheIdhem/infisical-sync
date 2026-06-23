package main

import "fmt"

const defaultBaseURL = "https://app.infisical.com"

var categoryEnvs = map[string][]string{
	"development": {"env1" , "env2"},
	"sandbox":     {"env3"},
	"production":  {"env4"},
}

var envAccountIDs = map[string]string{
	"env1": "1111111",
	"env2":   "22222222",
	"env3":   "3333333",
	"env4":    "4444444"
}

func categoryForEnv(env string) (string, error) {
	if _, isCategory := categoryEnvs[env]; isCategory {
		return env, nil
	}
	for category, envs := range categoryEnvs {
		for _, e := range envs {
			if e == env {
				return category, nil
			}
		}
	}
	return "", fmt.Errorf("unknown env %q: must be one of %v", env, allEnvs())
}

func isCategoryEnv(env string) bool {
	_, ok := categoryEnvs[env]
	return ok
}

func allEnvs() []string {
	var all []string
	for cat := range categoryEnvs {
		all = append(all, cat)
	}
	for _, envs := range categoryEnvs {
		all = append(all, envs...)
	}
	return all
}

func regionForEnv(env string) string {
	switch env {
	case "env1", "env2":
		return "us-west-2"
	default:
		return "us-east-1"
	}
}

func roleARNForEnv(env, roleName string) (string, error) {
	accountID, ok := envAccountIDs[env]
	if !ok {
		return "", fmt.Errorf("no AWS account ID mapped for env %q", env)
	}
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName), nil
}
