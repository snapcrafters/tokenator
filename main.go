package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/snapcrafters/tokenator/internal/config"
	"github.com/snapcrafters/tokenator/internal/tokenator"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version string = "dev"
	commit  string = "dev"

	repositories []string
	verbose      bool
)

var shortDesc = "A utility for distributing credentials to Snapcrafters repositories."
var longDesc string = `A utility for distributing credentials to Snapcrafters repositories.

tokenator provides automation for:

	- Generating Snap store tokens for publishing and promotion of snaps
	- Generating Github Personal Access Tokens
	- Accepting Github Personal Access Token requests against an org
	- Setting secrets across different Github environments

This tool is configured using a single file in one of the three following locations:

	- ./tokenator.yaml
	- $HOME/.config/tokenator/tokenator.yaml
	- /etc/tokenator/tokenator.yaml

For more details on the configuration format, see the homepage below.

The following environment variables must be set:

	- TOKENATOR_SNAPCRAFTERS_ORG_PAT - Github Personal Access Token with Snapcrafters org privileges
	- TOKENATOR_SNAPCRAFT_LOGIN - Snap Store login
	- TOKENATOR_SNAPCRAFT_PASSWORD - Snap Store password
	- TOKENATOR_LP_AUTH - Launchpad Remote Build auth file contents
	- TOKENATOR_SNAPCRAFTERS_BOT_LOGIN - Github login for the "snapcrafters-bot" user
	- TOKENATOR_SNAPCRAFTERS_BOT_PASSWORD - Github password for the "snapcrafters-bot" user
	- TOKENATOR_APP_ID  - ID of the Github app
	- TOKENATOR_APP_SECRET - Client secret for the Github app

For more information, visit the homepage at: https://github.com/snapcrafters/tokenator
`

var rootCmd = &cobra.Command{
	Use:           "tokenator",
	Version:       fmt.Sprintf("%s (%s)", version, commit),
	Short:         shortDesc,
	Long:          longDesc,
	SilenceErrors: false,
	SilenceUsage:  true,

	RunE: func(cmd *cobra.Command, args []string) error {
		tokenator.SetupLogger(verbose)

		cfg, err := parseConfig()
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}

		creds, err := parseCreds()
		if err != nil {
			return fmt.Errorf("failed to parse credentials: %w", err)
		}

		mgr := tokenator.NewManager(*cfg, creds)

		err = mgr.Process(repositories)
		if err != nil {
			slog.Error(err.Error())
		}

		return nil
	},
}

func main() {
	// Set the default config file name/type.
	viper.SetConfigName("tokenator")
	viper.SetConfigType("yaml")

	// Add some default config paths.
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.config/tokenator")
	viper.AddConfigPath("/etc/tokenator")

	// Setup environment variable parsing.
	viper.SetEnvPrefix("TOKENATOR")

	rootCmd.Flags().StringSliceVarP(&repositories, "repos", "r", []string{}, "comma-separated subset of repos to process. If omitted all configured repos will be processed.")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	err := rootCmd.Execute()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// parseCreds ensures that all required credentials are set and returns them
// in a format that can be passed to the manager.
func parseCreds() (config.Credentials, error) {
	requiredCredentials := []string{
		"snapcraft_login",
		"snapcraft_password",
		"snapcrafters_org_pat",
		"snapcrafters_bot_login",
		"snapcrafters_bot_password",
		"app_id",
		"app_secret",
		"lp_auth",
	}

	for _, cred := range requiredCredentials {
		viper.MustBindEnv(cred)
	}

	creds := config.Credentials{
		GithubToken: viper.GetString("snapcrafters_org_pat"),
		Launchpad:   viper.GetString("lp_auth"),
		SnapStore: config.LoginCredentials{
			Login:    viper.GetString("snapcraft_login"),
			Password: viper.GetString("snapcraft_password"),
		},
		Bot: config.LoginCredentials{
			Login:    viper.GetString("snapcrafters_bot_login"),
			Password: viper.GetString("snapcrafters_bot_password"),
		},
		GithubApp: config.GithubAppCredentials{
			ID:     viper.GetInt("app_id"),
			Secret: viper.GetString("app_secret"),
		},
	}

	return creds, nil
}

// parseConfig reads in the config and parses it into the correct format
func parseConfig() (*config.Config, error) {
	err := viper.ReadInConfig()
	if err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			return nil, errors.New("no config file found, see 'tokenator --help' for details")
		}
		return nil, errors.New("error parsing tokenator config file")
	}

	conf := &config.Config{}
	err = viper.Unmarshal(conf)
	if err != nil {
		return nil, errors.New("error parsing tokenator config file")
	}

	return conf, nil
}
