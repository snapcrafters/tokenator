package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/snapcrafters/tokenator/internal/config"
)

// OrgClient is used for making administrative changes to a given Github org.
type OrgClient struct {
	githubClient *github.Client
	org          string
	credentials  config.GithubAppCredentials
	token        string
}

// NewOrgClient constructs a new OrgClient using the supplied credentials.
func NewOrgClient(credentials config.GithubAppCredentials, org string) *OrgClient {
	return &OrgClient{
		githubClient: nil,
		org:          org,
		credentials:  credentials,
	}
}

// ApprovePATRequest approves a waiting request for access for a token for a specific snap.
func (oc *OrgClient) ApprovePATRequest(ctx context.Context, repo string) error {
	client, err := oc.client()
	if err != nil {
		return fmt.Errorf("unable to get org client: %w", err)
	}

	requestId, err := oc.findPATRequest(ctx, repo)
	if err != nil {
		return fmt.Errorf("could not find PAT request for %s/%s", oc.org, repo)
	}

	opts := github.ReviewPersonalAccessTokenRequestOptions{Action: "approve"}

	_, err = client.Organizations.ReviewPersonalAccessTokenRequest(ctx, oc.org, requestId, opts)
	if err != nil {
		return fmt.Errorf("failed to approve personal access token request: %w", err)
	}

	return nil
}

// client returns an authenticated Github client, generating an access token from
// the app credentials if the client hasn't previously been logged in.
func (oc *OrgClient) client() (*github.Client, error) {
	// Check if the client has already been initialised and just return it if it has.
	if oc.githubClient != nil {
		return oc.githubClient, nil
	}

	// Generate an access token from the app ID & client secret
	token, err := GetAppToken(oc.credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create token for Github app: %w", err)
	}

	oc.token = token
	return github.NewClient(nil).WithAuthToken(token), nil
}

// findPATRequest is used to find the ID of the latest PAT request for a given repo.
func (oc *OrgClient) findPATRequest(ctx context.Context, repo string) (int64, error) {
	reqs, err := oc.listPATRequests(ctx)
	if err != nil {
		return -1, fmt.Errorf("failed to list PAT requests: %w", err)
	}

	for _, req := range reqs {
		repos, err := oc.listPATRequestRepositories(ctx, req)
		if err != nil {
			return -1, fmt.Errorf("failed to list PAT request repositories: %w", err)
		}

		// tokenator generated requests only ever contain two repos
		if len(repos) != 2 {
			continue
		}

		containsSnapRepo := slices.ContainsFunc(repos, func(r *github.Repository) bool {
			return *(r.FullName) == fmt.Sprintf("%s/%s", oc.org, repo)
		})

		containsScreenshotRepo := slices.ContainsFunc(repos, func(r *github.Repository) bool {
			return *(r.FullName) == fmt.Sprintf("%s/ci-screenshots", oc.org)
		})

		if containsScreenshotRepo && containsSnapRepo {
			return int64(req.ID), nil
		}
	}

	return -1, fmt.Errorf("could not find PAT request for %s/%s", oc.org, repo)
}

// listPATrequests lists all of the PAT requests currently outstanding against the org.
func (oc *OrgClient) listPATRequests(ctx context.Context) ([]patRequest, error) {
	client := http.Client{}

	url := fmt.Sprintf("https://api.github.com/orgs/%s/personal-access-token-requests", oc.org)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct PAT list request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", oc.token))
	req.Header.Add("Accept", "application/vnd.github.json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET Github PAT list endpoint: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var ghPATReqs []patRequest
	err = json.Unmarshal(respBytes, &ghPATReqs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal PAT request: %w", err)
	}

	return ghPATReqs, nil
}

// listPATRequestRepositories gets the list of repositories a given PAT request relates to.
func (oc *OrgClient) listPATRequestRepositories(ctx context.Context, patReq patRequest) ([]*github.Repository, error) {
	client := http.Client{}

	req, err := http.NewRequest("GET", patReq.RepositoriesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct PAT request repo list request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", oc.token))
	req.Header.Add("Accept", "application/vnd.github.json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET Github PAT request repo list endpoint: %w", err)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var repos []*github.Repository
	err = json.Unmarshal(respBytes, &repos)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal PAT repos request: %w", err)
	}

	return repos, err
}

// patRequest represents the form of a PAT request as returned by the Github API
type patRequest struct {
	ID                  int         `json:"id"`
	CreatedAt           time.Time   `json:"created_at"`
	Owner               github.User `json:"owner"`
	Reason              interface{} `json:"reason"`
	RepositoriesURL     string      `json:"repositories_url"`
	RepositorySelection string      `json:"repository_selection"`
	TokenExpired        bool        `json:"token_expired"`
	TokenExpiresAt      time.Time   `json:"token_expires_at"`
	TokenLastUsedAt     interface{} `json:"token_last_used_at"`

	Permissions struct {
		Repository struct {
			// TODO: There are many, many more options here.
			Contents string `json:"contents"`
			Metadata string `json:"metadata"`
		} `json:"repository"`
	} `json:"permissions"`
}
