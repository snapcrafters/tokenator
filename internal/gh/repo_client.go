package gh

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/google/go-github/v58/github"
	"github.com/snapcrafters/tokenator/internal/config"
	"golang.org/x/crypto/nacl/box"
)

// RepoClient is used for manipulating secrets and environments in Github repositories.
type RepoClient struct {
	client *github.Client
	org    string
}

// NewRepoClient constructs a new RepoClient with the specified credentials.
func NewRepoClient(token string, org string) *RepoClient {
	return &RepoClient{
		client: github.NewClient(nil).WithAuthToken(token),
		org:    org,
	}
}

// SetEnvSecret sets a secret in the specified environment for the specified repo. If the
// environment does not exist, it is created.
func (rc *RepoClient) SetEnvSecret(ctx context.Context, repo string, track config.Track, secretName, secretValue string) error {
	r, _, err := rc.client.Repositories.Get(ctx, rc.org, repo)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	err = rc.ensureEnvironment(ctx, repo, track)
	if err != nil {
		return fmt.Errorf("failed to get environment: %w", err)
	}

	secret, err := rc.encryptSecret(ctx, r, track.Environment, secretName, secretValue)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	_, err = rc.client.Actions.CreateOrUpdateEnvSecret(ctx, int(*r.ID), track.Environment, secret)
	if err != nil {
		return fmt.Errorf("failed to set secret in environment: %w", err)
	}

	return nil
}

// encryptSecret fetches the public key from the specified Environment, and uses it to encrypt
// the specified secretValue such that it can be uploaded securely.
func (rc *RepoClient) encryptSecret(ctx context.Context, repo *github.Repository, envName, secretName, secretValue string) (*github.EncryptedSecret, error) {
	key, _, err := rc.client.Actions.GetEnvPublicKey(ctx, int(*repo.ID), envName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment public key: %w", err)
	}

	// Decode the public key from base64
	keyBytes, err := base64.StdEncoding.DecodeString(*key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	// Encrypt the secret value
	encrypted, err := box.SealAnonymous(nil, []byte(secretValue), (*[32]byte)(keyBytes), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	return &github.EncryptedSecret{
		Name:           secretName,
		KeyID:          *key.KeyID,
		EncryptedValue: base64.StdEncoding.EncodeToString(encrypted),
	}, nil
}

// ensureEnvironment attempts to fetch the specified Environment for the specified repo, and
// creates it if it doesn't exist.
func (rc *RepoClient) ensureEnvironment(ctx context.Context, repo string, track config.Track) error {
	_, resp, err := rc.client.Repositories.GetEnvironment(ctx, rc.org, repo, track.Environment)

	if resp.StatusCode == http.StatusNotFound {
		err = rc.createEnvironment(ctx, repo, track)
		if err != nil {
			return fmt.Errorf("failed to create environment: %w", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get environment: %w", err)
	}

	return nil
}

// createEnvironment creates an environment for the specified repository
func (rc *RepoClient) createEnvironment(ctx context.Context, repo string, track config.Track) error {
	t := true
	f := false

	createArgs := &github.CreateUpdateEnvironment{
		CanAdminsBypass: &t,
		DeploymentBranchPolicy: &github.BranchPolicy{
			CustomBranchPolicies: &t,
			ProtectedBranches:    &f,
		},
		PreventSelfReview: &t,
	}

	_, _, err := rc.client.Repositories.CreateUpdateEnvironment(ctx, rc.org, repo, track.Environment, createArgs)
	if err != nil {
		return fmt.Errorf("failed to create branch policy: %w", err)
	}

	branchPolicyRequest := &github.DeploymentBranchPolicyRequest{Name: &track.Branch}

	_, _, err = rc.client.Repositories.CreateDeploymentBranchPolicy(ctx, rc.org, repo, track.Environment, branchPolicyRequest)
	if err != nil {
		return fmt.Errorf("failed to create branch policy for environment: %w", err)
	}

	return nil
}
