# tokenator

A utility for distributing credentials to Snapcrafters repositories.

The Snapcrafters maintain many, many Snaps. In order to keep the maintenance sustainable and the contribution barrier low, much of the build, release and promotion pipeline is automated (see [snapcrafters/ci](https://github.com/snapcrafters/ci)). As part of this, and in order to keep elevated credentials safe, each repository is configured with a selection of low-privilege tokens that are specific to that repository and snap.

For each repository where the automation is active, the default branch is named `candidate` - representing the code for the Snap released on the `latest/candidate` channel. There is a Github Environment named `Candidate Branch` which contains the secrets listed in the table below:

| Credential name           | Issuer    | Scope                                                                                                      | Purpose                                                                                                                         |
| ------------------------- | --------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `SNAP_STORE_CANDIDATE`    | Snapcraft | `package_access`,`package_push`,`package_update`,`package_release`                                         | Upload and release a given Snap to the `latest/candidate` channel                                                               |
| `SNAP_STORE_STABLE`       | Snapcraft | `package_access`,`package_release`                                                                         | Promote tested revisions for a given Snap to the `latest/stable` channel                                                        |
| `LP_BUILD_SECRET`         | Launchpad | Execute remote builds                                                                                      | Enable Github Actions to send build jobs to the Launchpad build farm for all of the supported architectures.                    |
| `SNAPCRAFTERS_BOT_COMMIT` | Github    | Push changes to a given Snap repo, and to [ci-screenshots](https://github.com/snapcrafters/ci-screenshots) | Enable automated version bumps, automated release tagging and the publishing of screenshots collected during automated testing. |

## Challenges

Along the way to automating this, there were a few challenges:

1. While `snapcraft` makes provision for exporting tightly scoped credentials with `snapcraft export-login`, it cannot do this in a non-interactive environment, making it hard to automate
2. Github does not provide any API for creating, listing or otherwise managing Personal Access Tokens
3. While Github do provide APIs for managing the approval of Personal Access Token requests against a Github Organisation, this API is only accessible to Github Applications

Tokenator solves the above challenges by providing:

- A lightweight implementation of the Snap Store login flow which can be driven non-interactively
- An API for programmatically listing, creating and deleting fine-grained Personal Access Tokens
- An API for listing and accepting Personal Access Token requests against an organisation

## Credentials

To run Tokenator, the following environment variables need to be set:

- `TOKENATOR_SNAPCRAFTERS_ORG_PAT` - Github Personal Access Token with Snapcrafters org privileges
- `TOKENATOR_SNAPCRAFT_LOGIN` - Snap Store login
- `TOKENATOR_SNAPCRAFT_PASSWORD` - Snap Store password
- `TOKENATOR_LP_AUTH` - Launchpad Remote Build auth file contents
- `TOKENATOR_SNAPCRAFTERS_BOT_LOGIN` - Github login for the "snapcrafters-bot" user
- `TOKENATOR_SNAPCRAFTERS_BOT_PASSWORD` - Github password for the "snapcrafters-bot" user
- `TOKENATOR_SNAPCRAFTERS_TOTP_SECRET` - Github TOTP secret for the "snapcrafters-bot" user
- `TOKENATOR_APP_ID` - ID of the Github app
- `TOKENATOR_APP_SECRET` - Client secret for the Github app

## Config format

The config format is as follows:

```yaml
# (Required) The Github organisation where the Snap repositories are held.
org: <org>

# (Required) A list of Snap repos that need credentials.
snaps:
  # (Required) The name of the Snap, which should be the same as the repo name.
  - name: <snap name>
    # (Optional) A list of tracks to configure for the snap. This can be omitted and the
    # default will be the 'latest' track, with branch 'candidate' and env 'Candidate Branch'.
    tracks:
      # (Required) The name of the track. E.g. 'latest'.
      - name: <track name>
        # (Required) The name of the branch in the repo that maps to the track. E.g. 'candidate'.
        branch: <branch name>
        # (Required) The name of the Github Environment that has secrets for the track.
        environment: <environment name>
```

An example is as follows:

```yaml
org: snapcrafters
snaps:
  # Shorthand using default track info.
  - name: android-studio

  # Full config example with multiple tracks/branches.
  - gimp:
      tracks:
        - name: latest
          branch: candidate
          environment: Candidate Branch
        - name: preview
          branch: preview
          environment: Preview Candidate Branch
```

## Usage

The command line usage is as follows:

```
Usage:
  tokenator [flags]

Flags:
  -h, --help            help for tokenator
  -r, --repos strings   comma-separated list of repos to process
  -v, --verbose         enable verbose logging
      --version         version for tokenator
```

By default, running `./tokenator` will ensure that all configured repos are processed.

This can be reduced using `--repos/-r` like so:

```bash
# Just process the configured repo named "terraform"
./tokenator -r terraform

# Just process the configured repos named "terraform" and "gimp"
./tokenator -r terraform,gimp

```
