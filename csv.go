package main

import (
	"encoding/csv"
	"io"
	"os"
	"strings"
)

func parseCSV(path string) ([]CSVRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return nil, err
	}

	var rows []CSVRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 10 {
			continue
		}
		rows = append(rows, CSVRow{
			ID:                  strings.TrimSpace(rec[0]),
			VariableName:        strings.TrimSpace(rec[1]),
			SourceFile:          strings.TrimSpace(rec[2]),
			Environment:         strings.TrimSpace(rec[4]),
			RecommendedStore:    strings.TrimSpace(rec[8]),
			SuggestedAWSKeyPath: strings.TrimSpace(rec[9]),
		})
	}
	return rows, nil
}

func filterRows(rows []CSVRow, category string) []CSVRow {
	var out []CSVRow
	for _, r := range rows {
		if r.Environment == category || r.Environment == "shared" {
			out = append(out, r)
		}
	}
	return out
}
