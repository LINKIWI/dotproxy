package network

import (
	"math/rand"
	"time"
)

func init() {
	// Seed the random number generator once
	rand.Seed(time.Now().UnixNano())
}
