package main

import (
	"strings"
	"testing"
)

func TestGenerateRandomName(t *testing.T) {
	name := GenerateRandomName()
	if name == "" {
		t.Error("Generated name is empty")
	}
	// Check format somewhat (AdjectiveAnimalNumber)
	// It's hard to verify exactly without exposing arrays, but length > 0 is basic.
}

func TestGenerateBotName(t *testing.T) {
	name := GenerateBotName()
	if !strings.HasPrefix(name, "Bot ") {
		t.Errorf("Expected prefix 'Bot ', got %s", name)
	}
}

func TestGenerateBotNameWithPrefix(t *testing.T) {
	// Empty / whitespace-only prefix -> plain bot name.
	for _, empty := range []string{"", "   "} {
		if got := GenerateBotNameWithPrefix(empty); !strings.HasPrefix(got, "Bot ") {
			t.Errorf("empty prefix %q: expected plain 'Bot ...', got %q", empty, got)
		}
	}
	// Normal prefix -> "<prefix> Bot NNNN".
	if got := GenerateBotNameWithPrefix("Canary"); !strings.HasPrefix(got, "Canary Bot ") {
		t.Errorf("expected 'Canary Bot ...', got %q", got)
	}
}

func TestSanitizeNamePrefix(t *testing.T) {
	cases := map[string]string{
		"Canary":              "Canary",
		"  Canary  ":          "Canary",
		"Canary Build!":       "Canary Build",     // space kept, '!' dropped
		"Canary\n\t<script>":  "Canaryscript",     // control/invalid chars dropped, no space added
		"abcdefghijklmnopqrz": "abcdefghijklmnop",  // capped at 16 runes
	}
	for in, want := range cases {
		if got := sanitizeNamePrefix(in); got != want {
			t.Errorf("sanitizeNamePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
