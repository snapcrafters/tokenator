package tokenator

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/snapcrafters/tokenator/internal/config"
	"github.com/snapcrafters/tokenator/internal/gh"
	"github.com/snapcrafters/tokenator/internal/store"
)

// Manager is the engine behind Tokenator. It's responsible for iterating
// through the list of Snaps and ensuring they're populated with the correct
// secrets.
type Manager struct {
	id          string
	config      config.Config
	credentials config.Credentials

	orgClient   *gh.OrgClient
	patClient   *gh.PATClient
	repoClient  *gh.RepoClient
	storeClient *store.StoreClient
}

// NewManager constructs a new Manager configured with a set of snaps and credentials.
func NewManager(config config.Config, credentials config.Credentials) *Manager {
	return &Manager{
		id:          generateID(),
		config:      config,
		credentials: credentials,

		orgClient:   gh.NewOrgClient(credentials.GithubApp, config.Org),
		patClient:   gh.NewPATClient(credentials.Bot),
		repoClient:  gh.NewRepoClient(credentials.GithubToken, config.Org),
		storeClient: store.NewSnapStoreClient(credentials.SnapStore),
	}
}

// Process instructs the manager to iterate over the list of snaps it's configured
// with, optionally filtering the list to a subset.
func (m *Manager) Process(filter []string) error {
	ctx := context.Background()

	// Get the list of previously configured Personal Access Tokens, as some of these
	// will be deleted as they're superseded.
	pats, err := m.patClient.List("token8r")
	if err != nil {
		return fmt.Errorf("failed to list personal access tokens: %w", err)
	}

	for _, repo := range m.filterRepos(filter) {
		if len(repo.Tracks) == 0 {
			repo.SetDefaults()
		}

		for _, track := range repo.Tracks {
			// Generate the candidate store token and set it on Github
			err := m.setStoreSecret(ctx, repo.Name, track, "candidate")
			if err != nil {
				return fmt.Errorf("failed to set %s/candidate store secret: %w", track.Name, err)
			}

			// Generate the candidate store token and set it on Github
			err = m.setStoreSecret(ctx, repo.Name, track, "stable")
			if err != nil {
				return fmt.Errorf("failed to set %s/stable store secret: %w", track.Name, err)
			}

			// Set the Launchpad secret
			err = m.setLaunchpadSecret(ctx, repo.Name, track)
			if err != nil {
				return fmt.Errorf("failed to set Launchpad secret: %w", err)
			}

			// Generate the PAT
			err = m.setBotCommitSecret(ctx, repo.Name, track, pats)
			if err != nil {
				return fmt.Errorf("failed to set bot commit secret: %w", err)
			}
		}
	}

	return nil
}

// filterRepos takes a list of snap names and returns a list of only those Snaps
// from the manager's config.
func (m *Manager) filterRepos(filter []string) []config.Snap {
	repos := m.config.Repos
	if len(filter) > 0 {
		filteredSnaps := []config.Snap{}
		for _, repo := range repos {
			if slices.Contains(filter, repo.Name) {
				filteredSnaps = append(filteredSnaps, repo)
			}
		}
		repos = filteredSnaps
	}
	return repos
}

// setLaunchpadSecret is helper that sets the LP_BUILD_SECRET for a given snap/track/environment.
func (m *Manager) setLaunchpadSecret(ctx context.Context, snap string, track config.Track) error {
	err := m.repoClient.SetEnvSecret(ctx, snap, track, "LP_BUILD_SECRET", m.credentials.Launchpad)
	if err != nil {
		return fmt.Errorf("failed to set LP_BUILD_SECRET secret: %w", err)
	}

	fullName := fmt.Sprintf("%s/%s", m.config.Org, snap)
	slog.Info("secret set", "repo", fullName, "secret_name", "LP_BUILD_SECRET", "environment", track.Environment)

	return nil
}

// setLaunchpadSecret is helper that generates and sets the store secret for a given snap/track/environment.
func (m *Manager) setStoreSecret(ctx context.Context, snap string, track config.Track, channel string) error {
	token, err := m.storeClient.GenerateStoreToken(snap, track.Name, channel)
	if err != nil {
		return err
	}

	secretName := fmt.Sprintf("SNAP_STORE_%s", strings.ToUpper(channel))

	err = m.repoClient.SetEnvSecret(ctx, snap, track, secretName, token)
	if err != nil {
		return fmt.Errorf("failed to set %s secret: %w", secretName, err)
	}

	fullName := fmt.Sprintf("%s/%s", m.config.Org, snap)
	slog.Info("secret set", "repo", fullName, "secret_name", secretName, "environment", track.Environment)

	return nil
}

// setLaunchpadSecret is helper that generates and sets the bot commit secret for a given snap/track/environment.
func (m *Manager) setBotCommitSecret(ctx context.Context, snap string, track config.Track, pats []*gh.PAT) error {
	fullName := fmt.Sprintf("%s/%s", m.config.Org, snap)

	tokenRepos := []string{fullName, "snapcrafters/ci-screenshots"}

	// Create the access token on Github, which triggers a PAT approval in the org
	pat, err := m.patClient.Create(fmt.Sprintf("token8r-%s-%s-%s", m.id, snap, track.Name), tokenRepos, m.config.Org)
	if err != nil {
		return fmt.Errorf("failed to create personal access token: %w", err)
	}

	// Approve the PAT request we just triggered so the new token is active
	err = m.orgClient.ApprovePATRequest(ctx, snap)
	if err != nil {
		return fmt.Errorf("failed to approve personal access token request: %w", err)
	}

	// Set the SNAPCRAFTERS_BOT_COMMIT secret
	err = m.repoClient.SetEnvSecret(ctx, snap, track, "SNAPCRAFTERS_BOT_COMMIT", pat.Token)
	if err != nil {
		return fmt.Errorf("failed to set SNAPCRAFTERS_BOT_COMMIT secret: %w", err)
	}

	slog.Info("secret set", "repo", fullName, "secret_name", "SNAPCRAFTERS_BOT_COMMIT", "environment", track.Environment)

	// Iterate through the list of PATs, cleaning up redundant secrets where necessary
	for _, pat := range pats {
		patSuffix := fmt.Sprintf("%s-%s", snap, track.Name)
		// If the token name contains the same suffix, but doesn't contain the ID of the
		// manager, then it was created by a prior run and is now unneeded, so can be
		// deleted.
		if strings.Contains(pat.Name, patSuffix) && !strings.Contains(pat.Name, m.id) {
			err := pat.Delete(m.patClient)
			if err != nil {
				return fmt.Errorf("failed to delete personal access token: %w", err)
			}
		}
	}

	return nil
}

// generateID generates a sha256 hash from the current unix timestamp, and returns
// just the first 4 characters.
func generateID() string {
	h := sha256.New()

	value := fmt.Sprintf("%d", time.Now().Unix())
	h.Write([]byte(value))
	bs := h.Sum(nil)

	return fmt.Sprintf("%x", bs)[0:4]
}
