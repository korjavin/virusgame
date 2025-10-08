package main

import (
	"fmt"
	"math/rand"
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
