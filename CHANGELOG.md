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