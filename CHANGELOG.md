# Changelog

## v0.1.0

Initial version.

## v0.2.0

- Improved parser of existing config files.

- Added new configuration options
    - Switch to make dependabutler verify that a registry is really used, before adding it to the config.
    - List of files to search for the above, in addition to the manifest file itself.

- Ignoring archived and empty repositories.

- Updated sample config files (new options, improved patterns).

## v0.2.1

- Added manifest type specific configuration to allow overriding default update settings.

## v0.2.2

- Added config parameter for adding a random suffix to PR branch names (`branch-name-random-suffix`).
- Added check to avoid `insecure-external-code-execution` being set on invalid manifest types.
- Code cleanup.

## v0.3.0

- Added support for updates of existing PRs.

- Added config parameter for adding a sleep time after PR creation/update (`sleep-after-pr-action`).

- Fixes and code cleanup.

## v0.3.1

- Fixes.

## v0.3.2

- Fix incorrect path comparison when checking if manifest is covered by existing config.
- Process manifests sorted by path, to generate stable output.

## v0.3.3

- Fix addition of default registries.

## v0.4.0

- Update to Go 1.21.
- Added parsing of `enable-beta-ecosystems` and `groups` config properties.

## v0.5.0

- Update to Go 1.22.
- Added config property `manifest-ignore-pattern` to exclude directories from the manifest file search.

## v0.6.0

- Update to Go 1.24.
- Fail and stop in case a PR cannot be created.

## v0.6.2

- Added fixing existing updates.
- Fix an empty Directory values, setting them to /

## v0.7.0

- Added removing unused updates.
- Added support for the `directories` property.

## v0.7.1

- Added removing unused registries.

## v0.7.2

- Added `stable-group-prefixes` option (default: true) that ensures group names have unique numeric prefixes (01_, 02_, 03_, etc.).

## v0.7.3

- Fixed a bug related to the `directories` property.

## v0.8.0

- Added support for the `cooldown` property.
- Added configuration flag `update-missing-cooldown-settings` to update existing manifests with default settings for the `cooldown` property.

## v0.8.1

- Fixed `update-missing-cooldown-settings` to also take into account potential override settings for the `cooldown` property.

## v0.8.2

- Added deprecation support for the `reviewers` field in `dependabot.yml` (will be removed by GitHub in May 2025)

## v0.9.0

- Added support for zero value in `open-pull-requests-limit` configuration option to stop PRs from Dependabot.
- Added support for registry URL as environment variable (previously only username and password were supported).
- Added error handling to exit with status code 1 when errors occur during processing, ensuring GitHub Actions can detect failures.
- Added throttling mechanism for GitHub API calls with new `rateLimitBuffer` CLI parameter to prevent silent failures due to rate limit exhaustion.
- Migrated from unmaintained `gopkg.in/yaml.v3` to actively maintained `github.com/goccy/go-yaml` library, fixing emoji corruption issues.
