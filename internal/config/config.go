package config

// Config represents the top-level configuration structure for Tokenator.
type Config struct {
	Org   string `yaml:"org"`
	Repos []Repo `yaml:"repos"`
}

// Repo represents a repo for a given snap package which needs configuring.
type Repo struct {
	Name   string   `yaml:"name"`
	Snaps  []string `yaml:"snaps,omitempty"`
	Tracks []Track  `yaml:"tracks"`
}

// SetDefaults ensures that if no track information is specified for a given snap,
// sensible defaults are used.
func (s *Repo) SetDefaults() {
	s.Tracks = []Track{{Name: "latest", Branch: "candidate", Environment: "Candidate Branch"}}
}

// Track ties together a track in the store, with a branch in the repo, and an environment.
type Track struct {
	Name        string `yaml:"name"`
	Branch      string `yaml:"branch"`
	Environment string `yaml:"environment"`
}

// Credentials contains all of the credentials needed for Tokenator to function
type Credentials struct {
	// GithubToken PAT with privileges over the Snapcrafters org
	GithubToken string

	// Login details for the snapcraft.io store
	SnapStore LoginCredentials

	// Credentials for sending build jobs to Launchpad
	Launchpad string

	// Github Login for the 'snapcrafters-bot' account
	Bot LoginCredentials

	// App ID and Client Secret for the 'TOKENATORs' Github app
	GithubApp GithubAppCredentials
}

// LoginCredentials represents traditional username/password credentials
type LoginCredentials struct {
	Login      string
	Password   string
	TOTPSecret string
}

// GithubAppCredentials enable the representation of a Github App ID and client secret
// encoded in PEM format.
type GithubAppCredentials struct {
	ID     int
	Secret string
}
