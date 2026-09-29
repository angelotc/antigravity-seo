package main

import (
	"encoding/json"
	"testing"
)

func TestFindingsTrip(t *testing.T) {
	tests := []struct {
		threshold string
		sevs      []string
		want      bool
	}{
		{"", []string{"CRITICAL", "WARNING"}, false},
		{"critical", []string{"CRITICAL"}, true},
		{"critical", []string{"WARNING", "INFO", "PASS"}, false},
		{"warning", []string{"WARNING"}, true},
		{"warning", []string{"INFO", "PASS", "CRITICAL"}, true},
		{"warning", []string{"INFO", "PASS"}, false},
		{"CRITICAL", []string{"CRITICAL"}, true},
		{"Warning", []string{"warning"}, true},
		{"critical", nil, false},
	}
	for _, tc := range tests {
		got, err := findingsTrip(tc.threshold, tc.sevs)
		if err != nil || got != tc.want {
			t.Errorf("findingsTrip(%q, %v) = %v, %v; want %v", tc.threshold, tc.sevs, got, err, tc.want)
		}
	}
	if _, err := findingsTrip("bogus", []string{"CRITICAL"}); err == nil {
		t.Error("invalid threshold should return an error")
	}
	if _, err := findingsTrip("info", nil); err == nil {
		t.Error("info is not a valid threshold")
	}
}

func TestFormatErrorJSON(t *testing.T) {
	type envelope struct {
		Error map[string]string `json:"error"`
	}
	var e envelope
	out := formatErrorJSON("psi", "GOOGLE_API_KEY not set.\n  1. Create a key\n  2. export it\n")
	if err := json.Unmarshal(out, &e); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, out)
	}
	if e.Error["command"] != "psi" || e.Error["message"] != "GOOGLE_API_KEY not set." {
		t.Errorf("unexpected envelope: %s", out)
	}
	if e.Error["hint"] != "1. Create a key\n  2. export it" {
		t.Errorf("unexpected hint: %q", e.Error["hint"])
	}

	e = envelope{}
	if err := json.Unmarshal(formatErrorJSON("headers", "fetch error: boom"), &e); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Error["hint"]; ok {
		t.Errorf("hint should be omitted for single-line messages: %v", e.Error)
	}
}

func TestHasJSONFlag(t *testing.T) {
	for _, a := range [][]string{{"u", "--json"}, {"-json"}, {"--json=true"}, {"-json=true", "x"}} {
		if !hasJSONFlag(a) {
			t.Errorf("hasJSONFlag(%v) = false", a)
		}
	}
	for _, a := range [][]string{nil, {"--json=false"}, {"--jsonx"}, {"json"}} {
		if hasJSONFlag(a) {
			t.Errorf("hasJSONFlag(%v) = true", a)
		}
	}
}
