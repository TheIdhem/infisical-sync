package main

type CSVRow struct {
	ID                  string
	VariableName        string
	SourceFile          string
	Environment         string
	RecommendedStore    string
	SuggestedAWSKeyPath string
}

type SyncKey struct {
	SecretPath       string
	RecommendedStore string
}

type syncEntry struct {
	ID         string
	Name       string
	SecretPath string
	EnvName    string
}
