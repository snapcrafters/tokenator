package gh

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/snapcrafters/tokenator/internal/config"
	"github.com/tidwall/gjson"
)

// GetAppToken takes a Github App ID and Client Secret in PEM format as inputs,
// and returns an access token that can be used with the Github API.
func GetAppToken(credentials config.GithubAppCredentials) (string, error) {
	// Encode a JWT using the app ID and client secret such than an 'Authorization'
	// header can be constructed.
	jwt, err := encodeJWT(credentials.ID, credentials.Secret)
	if err != nil {
		return "", fmt.Errorf("failed to encode JWT for Github API: %w", err)
	}

	// Get the token endpoint for the specified app.
	accessTokensUrl, err := getAppTokenEndpoint(jwt)
	if err != nil {
		return "", fmt.Errorf("failed to get token endpoint for app: %w", err)
	}

	// Generate a token by posting to the accessTokensUrl with the JWT
	// as an authorization header.
	token, err := fetchAppToken(accessTokensUrl, jwt)
	if err != nil {
		return "", fmt.Errorf("failed to get token for app: %w", err)
	}

	return token, nil
}

// fetchAppToken sends a POST request to a Github App's access token URL,
// using a JWT as authorization, and returns a Github token that can be
// used with the Github API.
func fetchAppToken(url string, jwt string) (string, error) {
	client := http.Client{}
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to construct token request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", string(jwt)))
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to POST access token URL: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	token := gjson.GetBytes(respBytes, "token")
	if !token.Exists() {
		return "", fmt.Errorf("no access token found in response json")
	}

	return token.String(), nil
}

// getAppTokenEndpoint is a helper method for fetching the Access Token Endpoint for
// a given Github application.
func getAppTokenEndpoint(jwt string) (string, error) {
	client := http.Client{}
	req, err := http.NewRequest("GET", "https://api.github.com/app/installations", nil)
	if err != nil {
		return "", fmt.Errorf("failed to construct installations endpoint request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", string(jwt)))
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to GET Github app installations endpoint: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	accessTokensUrl := gjson.GetBytes(respBytes, "0.access_tokens_url")
	if !accessTokensUrl.Exists() {
		return "", fmt.Errorf("no access token URL found in response json")
	}

	return accessTokensUrl.String(), nil
}

// encodeJET is a helper method that forms a JWT suitable for authorization against
// the Github API for a given app ID/app secret combination.
func encodeJWT(appId int, appSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Duration(10) * time.Minute).Unix(),
		"iss": appId,
	})

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(appSecret))
	if err != nil {
		return "", fmt.Errorf("error parsing private key for JWT: %w", err)
	}

	jwt, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("error signing JWT: %w", err)
	}

	return jwt, nil
}
