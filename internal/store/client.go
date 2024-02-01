package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/snapcrafters/tokenator/internal/config"
	"github.com/tidwall/gjson"
	"gopkg.in/macaroon.v1"
)

// channelPermissions represents the set of ACLS applied to store tokens
// depending on which channel the token is for interacting with.
var channelPermissions map[string][]string = map[string][]string{
	"candidate": {"package_access", "package_push", "package_update", "package_release"},
	"stable":    {"package_access", "package_release"},
}

// StoreClient is a wrapper around http.Client for logging into a Canonical store.
type StoreClient struct {
	authEndpoints StoreAuthEndpoints
	client        *http.Client
	credentials   config.LoginCredentials
	endpoints     StoreEndpoints
}

// NewSnapStoreClient constructs a new StoreClient for interacting with the snap store.
func NewSnapStoreClient(credentials config.LoginCredentials) *StoreClient {
	return &StoreClient{
		endpoints:     SNAP_STORE_ENDPOINTS,
		authEndpoints: UBUNTU_ONE_SNAP_STORE_AUTH_ENDPOINTS,
		credentials:   credentials,
		client:        &http.Client{},
	}
}

// GenerateStoreToken takes a snap, track and channel and returns a token with a
// TTL of 1 year, with default permissions for the given channel.
func (sc *StoreClient) GenerateStoreToken(snap, track, channel string) (string, error) {
	permissions, ok := channelPermissions[channel]
	if !ok {
		return "", fmt.Errorf("invalid channel specified")
	}

	tokenParams := tokenParams{
		Permissions: permissions,
		Description: fmt.Sprintf("tokenator-%s-%s", snap, track),
		TTL:         60 * 60 * 24 * 365, // 1 year
		Credentials: sc.credentials,
		Packages:    []string{snap},
		Channels:    []string{fmt.Sprintf("%s/%s", track, channel)},
	}

	token, err := sc.login(tokenParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate store token: %w", err)
	}

	return token, err
}

// login is used to login to a Canonical store and generate a scoped token
// with access to the specified packages, at the specified permissions level.
func (sc *StoreClient) login(params tokenParams) (string, error) {
	tokenRequest := tokenRequest{
		Permissions: params.Permissions,
		Description: params.Description,
		TTL:         params.TTL,
		Packages:    []Package{},
		Channels:    params.Channels,
	}

	for _, p := range params.Packages {
		tokenRequest.Packages = append(tokenRequest.Packages, NewSnapPackage(p))
	}

	rootMacaroon, err := sc.getRootMacaroon(tokenRequest)
	if err != nil {
		return "", fmt.Errorf("failed to get root macaroon: %w", err)
	}

	dischargedMacaroon, err := sc.getDischargedMacaroon(rootMacaroon, params)
	if err != nil {
		return "", fmt.Errorf("failed to get discharged macaroon: %w", err)
	}

	token, err := NewUbuntuOneToken(rootMacaroon, dischargedMacaroon)
	if err != nil {
		return "", fmt.Errorf("failed to create a valid Ubuntu One token: %w", err)
	}

	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Ubuntu One to JSON: %w", err)
	}

	tokenEncoded := base64.StdEncoding.EncodeToString(tokenJSON)
	return tokenEncoded, nil
}

// getDischargedMacaroon is a helper function that returns a discharged macaroon from the
// store, given a root macaroon and some credentials.
func (sc *StoreClient) getDischargedMacaroon(root *macaroon.Macaroon, params tokenParams) (*macaroon.Macaroon, error) {
	u, _ := url.Parse(sc.authEndpoints.AuthURL)

	idx := slices.IndexFunc(root.Caveats(), func(c macaroon.Caveat) bool {
		return c.Location == u.Host
	})

	body := macaroonDischargeParams{
		Email:    params.Credentials.Login,
		Password: params.Credentials.Password,
		CaveatId: root.Caveats()[idx].Id,
	}

	resp, err := sc.post(sc.authEndpoints.AuthURL+sc.authEndpoints.TokensExchange, body)
	if err != nil {
		return nil, fmt.Errorf("failed to request token exchange endpoint: %w", err)
	}

	dischargedMacaroon, err := sc.deserializeMacaroon(resp, "discharge_macaroon")
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize macaroon: %w", err)
	}

	return dischargedMacaroon, nil
}

// getRootMacaroon is a helper function that returns a root macaroon from the store.
func (sc *StoreClient) getRootMacaroon(tr tokenRequest) (*macaroon.Macaroon, error) {
	resp, err := sc.post(sc.endpoints.BaseURL+sc.authEndpoints.Tokens, tr)
	if err != nil {
		return nil, fmt.Errorf("failed to request token exchange endpoint: %w", err)
	}

	rootMacaroon, err := sc.deserializeMacaroon(resp, "macaroon")
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize macaroon: %w", err)
	}

	return rootMacaroon, nil
}

// deserializeMacaroon is a helper function to take any response from the store
// which contains a macaroon, and deserialize it into a macaroon.Macaroon.
func (sc *StoreClient) deserializeMacaroon(resp *http.Response, field string) (*macaroon.Macaroon, error) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read macaroon response body: %w", err)
	}

	respMac := gjson.Get(string(respBytes), field)
	if !respMac.Exists() {
		return nil, fmt.Errorf("no macaroon found in response json")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(respMac.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode unmarshalled macaroon")
	}

	mac := &macaroon.Macaroon{}
	err = mac.UnmarshalBinary(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize macaroon: %w", err)
	}

	return mac, nil
}

// post is a helper function for making HTTP POST requests to the store with
// the correct headers set.
func (sc *StoreClient) post(url string, body any) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body to json: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to construct post request to url '%s': %w", url, err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	resp, err := sc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request url '%s': %w", url, err)
	}

	return resp, err
}
