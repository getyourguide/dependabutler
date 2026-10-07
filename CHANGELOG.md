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

## v0.9.4

- Added config parameter `pr-labels` to control the labels applied to PRs created by dependabutler (defaults to `dependabutler`).

## v0.9.5

- Fixed the API throttling introduced in v0.9.0: the remaining rate limit is now taken from the `X-RateLimit-*`
  headers of real API responses instead of the `GET /rate_limit` endpoint, which was observed reporting an unused
  budget while the enforced counter had already been spent. The tool now waits until the reported reset time rather
  than sleeping blindly, and retries a repository once if it ran into the limit while being processed. When a
  response reports the budget as fully used, the tool pauses proactively instead of running into the rejection.
- Upgraded `go-github` from v50 to v90. Among other fixes, the old version did not recognize rate limit rejections
  with HTTP status 429, which GitHub can return for both primary and secondary limits.
- Deprecated the `rateLimitBuffer` parameter: rate limits no longer need to be configured. The flag is still
  accepted so existing callers keep working, but it has no effect (a warning is logged) and will be removed in a
  future release.

## v0.10.0

- Update to Go 1.27.1
- Added `-mode=report`: prints one JSON line per repository with the manifests found, whether an update entry covers
  each of them, and the update entries with their schedule. Nothing is written to the repositories.
- A permission error when creating a pull request no longer stops the run: the repository is counted as failed and
  the next one is processed.
- Fixed `cronjob` being removed from `schedule` when `dependabot.yml` is rewritten, which left an invalid file for
  entries using `interval: cron`.
- A warning is logged when GitHub truncates the file list of a large repository, since manifests may be missed.
- Remote mode fails if `repoFile` cannot be read or lists no repository, instead of exiting successfully without doing
  anything. Blank lines in `repoFile` are skipped.

## v0.10.1

- Keys of `dependabot.yml` that dependabutler does not know (like `multi-ecosystem-groups`, `exclude-paths` or
  `group-by`) are kept when the file is rewritten, at every level. Before, they were removed. Unknown keys are
  written after the known ones of the same section.
- Fixed `replaces-base` being rewritten as the string `"true"` instead of the boolean `true`. Files that contain the
  string form are read, and written back as a boolean the next time dependabutler rewrites them.

## v0.11.0

- Added `schedule-slots`: new update entries get a twice-weekly `cron` schedule derived from the repository name, in
  one of several windows of hours. A window selects repositories by ruleset or by a file listing them. Off unless the
  configuration file has the block. See "Schedule slots" in the README.
- `-repo` sets the repository name used for schedule slots in local mode.

## v0.12.0

- Added `enforce`: sets `schedule`, `cooldown` and `open-pull-requests-limit` of existing update entries to the
  configured values. Repositories can keep their own values through an exceptions file, and enforcement can be limited
  to a list of repositories. Off unless the configuration file has the block. See "Enforcing settings on existing
  entries" in the README.
- Added `enforce-pr-title`, `enforce-commit-message` and `enforce-pr-note` to `pull-request-parameters`, used when
  enforced settings are the only change.
