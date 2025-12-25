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
