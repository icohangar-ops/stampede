// Package id mints unguessable ids and verification tokens.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns 32 hex characters (128 bits).
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Token is the secret a site owner places in a file or TXT record.
func Token() string {
	return "st_" + New()
}
