package store

import (
	"encoding/base64"
	"fmt"

	"github.com/snapcrafters/tokenator/internal/config"
	"gopkg.in/macaroon.v1"
)

// NewUbuntuOneToken constructs a valid UbuntuOneToken given a input root token, and
// discharged token.
func NewUbuntuOneToken(rootMacaroon *macaroon.Macaroon, dischargedMacaroon *macaroon.Macaroon) (*UbuntuOneToken, error) {
	binaryDischargedMacaroon, err := dischargedMacaroon.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal discharged macaroon to binary format: %w", err)
	}

	binaryRootMacaroon, err := rootMacaroon.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal root macaroon to binary format: %w", err)
	}

	return &UbuntuOneToken{
		TokenType: "u1-macaroon",
		UbuntuOneMacaroons: ubuntuOneMacaroons{
			RootMacaroon:       base64.RawURLEncoding.EncodeToString(binaryRootMacaroon),
			DischargedMacaroon: base64.RawURLEncoding.EncodeToString(binaryDischargedMacaroon),
		},
	}, nil
}

// UbuntuOneToken represents the top-level token object that is returned to the client
// encoded in base64 for use with CLI tools.
type UbuntuOneToken struct {
	TokenType          string             `json:"t"`
	UbuntuOneMacaroons ubuntuOneMacaroons `json:"v"`
}

// ubuntuOneMacaroons represents the two macaroons that are present in a UbuntuOneToken.
type ubuntuOneMacaroons struct {
	RootMacaroon       string `json:"r"`
	DischargedMacaroon string `json:"d"`
}

// tokenRequest contains the fields needed when making a request for the root
// macaroon from a Canonical store.
type tokenRequest struct {
	Permissions []string  `json:"permissions"`
	Description string    `json:"description"`
	TTL         int       `json:"ttl"`
	Packages    []Package `json:"packages"`
	Channels    []string  `json:"channels"`
}

// tokenParams is a data structure containing all the fields required to login to a
// Canonical store and discharge a macaroon.
type tokenParams struct {
	Channels    []string
	Credentials config.LoginCredentials
	Description string
	Packages    []string
	Permissions []string
	TTL         int
}

// macaroonDischargeParams represents the fields required in order to discharge a macaroon.
type macaroonDischargeParams struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	CaveatId string `json:"caveat_id"`
}
