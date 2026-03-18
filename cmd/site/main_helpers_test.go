package main

import (
	"reflect"
	"testing"
)

func TestParseCORSOrigins(t *testing.T) {
	got := parseCORSOrigins(" https://a.example , ,https://b.example,https://a.example ")
	want := map[string]bool{
		"https://a.example": true,
		"https://b.example": true,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCORSOrigins() = %#v, want %#v", got, want)
	}
}

func TestParseAllowedHosts(t *testing.T) {
	got := parseAllowedHosts(" GitHub.com , ,EXAMPLE.com,github.com ")
	want := []string{"github.com", "example.com", "github.com"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAllowedHosts() = %#v, want %#v", got, want)
	}
}

func TestParseAllowedHostsEmpty(t *testing.T) {
	if got := parseAllowedHosts(""); got != nil {
		t.Fatalf("parseAllowedHosts(\"\") = %#v, want nil", got)
	}
}
