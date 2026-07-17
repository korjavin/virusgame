package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var adjectives = []string{
	"Brave", "Clever", "Wild", "Swift", "Bold", "Mighty", "Mystic", "Noble",
	"Fierce", "Gentle", "Silent", "Rapid", "Calm", "Proud", "Wise", "Happy",
	"Lucky", "Sneaky", "Cunning", "Bright", "Dark", "Golden", "Silver", "Royal",
	"Ancient", "Modern", "Quick", "Slow", "Tiny", "Giant", "Cool", "Hot",
}

var animals = []string{
	"Octopus", "Tiger", "Phoenix", "Dragon", "Eagle", "Wolf", "Bear", "Fox",
	"Lion", "Hawk", "Shark", "Panther", "Raven", "Falcon", "Cobra", "Viper",
	"Lynx", "Owl", "Dolphin", "Whale", "Rhino", "Jaguar", "Cheetah", "Leopard",
	"Puma", "Otter", "Badger", "Raccoon", "Moose", "Buffalo", "Bison", "Elk",
}

var rng *rand.Rand

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// GenerateRandomName creates a random username in format: AdjectiveAnimalNumber
func GenerateRandomName() string {
	adjective := adjectives[rng.Intn(len(adjectives))]
	animal := animals[rng.Intn(len(animals))]
	number := rng.Intn(100)
	return fmt.Sprintf("%s%s%d", adjective, animal, number)
}

// GenerateBotName creates a bot name in format: Bot 1234
func GenerateBotName() string {
	number := rng.Intn(9000) + 1000 // 1000 to 9999
	return fmt.Sprintf("Bot %d", number)
}

// GenerateBotNameWithPrefix behaves like GenerateBotName but prepends a caller-
// supplied (sanitized) prefix, e.g. prefix "Canary" -> "Canary Bot 1234". Used
// so a canary bot-hoster's identities are clearly distinguishable in the lobby
// and in last_games telemetry. An empty/unusable prefix yields a plain bot name.
func GenerateBotNameWithPrefix(prefix string) string {
	name := GenerateBotName()
	if p := sanitizeNamePrefix(prefix); p != "" {
		return p + " " + name
	}
	return name
}

// sanitizeNamePrefix keeps only display-safe characters (letters, digits, space,
// dash), capped at 16 runes, so a prefix arriving over the wire can never inject
// control characters or unbounded length into a username.
func sanitizeNamePrefix(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if b.Len() >= 16 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '-':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
