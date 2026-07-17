package main

import "crypto/rand"

func fillRandom(b []byte) { _, _ = rand.Read(b) }
