# dependabutler

<img alt="dependabutler logo" src="dependabutler.png" style="width:48px"/>

Automatically create or update the `dependabot.yml` config file of GitHub repositories, based on manifest files present.


## Installation

```
go install github.com/getyourguide/dependabutler/cmd/dependabutler@latest
```

## Usage

### Configuration file
The default configuration file name is `dependabutler.yml`. Use `dependabutler-sample.yml` as a starting point and for reference.

### Rewriting dependabot.yml
When dependabutler changes a `dependabot.yml`, it writes the whole file again. Every key is kept, including keys
dependabutler does not know, which are written after the known ones of the same section. Update entries are sorted by
ecosystem and directory. A directory listed in `directories` that no longer exists is removed, like an entry whose
`directory` no longer exists. Groups keep the order they are written in, since Dependabot puts a dependency in the first
group it matches. Comments are not kept.

### Groups
`update-defaults` and `update-overrides` can set `groups` for new update entries. The groups of an override replace the
default groups as a whole:

```yaml
update-defaults:
  groups:
    minor-patch: {patterns: ["*"], update-types: [minor, patch]}
update-overrides:
  npm:
    groups:
      minor-patch: {patterns: ["*"], update-types: [minor, patch]}
      development-major: {dependency-type: development, update-types: [major]}
```

Keys of a group that dependabutler does not know are dropped with a warning, so a typo is not written into every
repository. The configuration file is rejected when:
- a group has `dependency-type` in `update-defaults`, or in `update-overrides` of an ecosystem other than bundler,
  composer, maven, mix, npm and pip, which Dependabot supports it for;
- a group matching every dependency (`patterns: ["*"]` and nothing else) comes before another group that applies to the
  same updates: Dependabot puts a dependency in the first group it matches, so the later group would stay empty.

### Directory grouping
In a repository with several directories for the same ecosystem, `directory-grouping` chooses how they are split into
update entries:

- `global`: one entry for all directories (`directories:`), so each group gets one pull request for all of them.
- `per-app`: one entry per directory, so each group gets one pull request per directory.

```yaml
directory-grouping:
  default: global
  property: dependabot-grouping   # optional custom property of a repository, with the value global or per-app
```

- A repository uses the value of its custom property if that is a mode, otherwise `default`. Teams can switch their
  repository in its settings. The property is read with one API call per repository, which needs read access to custom
  properties; if it cannot be read, the repository is skipped and counted as failed. In local mode, `default` is used.
- In `global` mode, the directory of a new manifest is added to an entry of its ecosystem whose settings are the same
  as those of a new entry, apart from directories and registries. If there is none, it gets its own entry.
- Existing entries are changed only when `enforce` lists `directory-grouping`. In `global` mode, entries of an
  ecosystem that have the same settings apart from directories and registries are merged, and their registries are
  combined. Entries with other differences, for example in `ignore`, `commit-message` or `target-branch`, stay
  separate. In `per-app` mode, entries with several directories are split into one entry per directory, unless another
  entry already has one of those directories; directories with a glob pattern are left as they are.

### Schedule slots
With `schedule-slots` in the configuration file, new update entries do not get the `schedule` of `update-defaults`.
Each repository gets its own slot instead, so that Dependabot does not run for all repositories at the same time:

```yaml
schedule:
  interval: cron
  cronjob: "0 4 * * 2,5"
  timezone: Europe/Berlin
```

Every repository runs twice a week, at the same hour, on a weekday and on the weekday three weekdays later (Monday and
Thursday, Tuesday and Friday, Wednesday and Monday, Thursday and Tuesday, Friday and Wednesday). Each weekday is part of
exactly two pairs, so every weekday gets about the same number of repositories.

```yaml
schedule-slots:
  salt: "01J9Z3K4M5N6P7Q8R9S0T1V2W3"
  windows:
    - name: office-hours
      hours: [10, 11, 13, 14, 15]
      timezone: Europe/Berlin
      rulesets: [audited]
      repos-file: office-hours-repos.txt
    - name: early
      hours: [2, 3, 4, 5, 6, 7, 8]
      timezone: Europe/Berlin
```

- A repository uses the first window that selects it: an active ruleset in `rulesets` applies to it (organization
  rulesets included), or `repos-file` lists it. The last window has neither and holds every other repository.
- `repos-file` is relative to the configuration file and lists one repository name per line. Blank lines are skipped.
- Rulesets are read with one API call per repository, only when a window uses `rulesets`. If they cannot be read, the
  repository is skipped and counted as failed, rather than placed in the wrong window. In local mode rulesets are not
  read, and the repository name is `repo`, or the name of `dir` if `repo` is not set.
- The slot is `FNV-1a 32(salt + ":" + repo) mod (5 × number of hours)` of the window, with `repo` in lower case: the
  weekday pair is the result divided by the number of hours, the hour is the remainder. Repository names in
  `repos-file` also match regardless of case. Adding or removing a repository does not move any other
  one. Hashing does not spread repositories perfectly evenly: to reduce the busiest slot, try many salts against the
  real repository names once, and keep the best. Changing the salt moves every repository.
- Only new update entries get a slot. A schedule set in `update-overrides` is ignored for them, with a warning.
  Existing entries keep their schedule.

### Enforcing settings on existing entries
By default, dependabutler applies the settings of the configuration file only to update entries it creates. With
`enforce`, it also sets the listed fields of existing entries to the value a new entry of the same ecosystem would get:
`update-defaults`, then `update-overrides`, then the schedule slot.

```yaml
enforce:
  fields: [schedule, cooldown, open-pull-requests-limit]
  exceptions-file: enforce-exceptions.yml
  repos-file: enforce-repos.txt
```

- `fields` can hold `schedule`, `cooldown`, `open-pull-requests-limit`, `groups` and `directory-grouping`. Each field is
  replaced as a whole, so a `cooldown` also gets the configured `include` and `exclude` lists. Every other key is left
  as it is. `directory-grouping` merges or splits entries as described in "Directory grouping".
- An entry is changed only if a field differs, so a repository that already matches gets no pull request.
- `exceptions-file` lists repositories that keep their own values for some fields. `repo`, `fields`, `reason` and
  `owner` are required:
  ```yaml
  - repo: some-repo
    fields: [cooldown]
    reason: why the repository needs its own values
    owner: the team responsible for it
  ```
- `repos-file` limits enforcement to the listed repositories, one per line, for example to roll it out in steps. Without
  it, every repository is enforced. Other changes, like new update entries, still apply to every repository.
- Both files are relative to the configuration file. Repository names match regardless of case.
- Each enforced field needs a value to enforce: the schedule needs `schedule-slots` or a `schedule` in
  `update-defaults`; the cooldown, the open pull request limit and the groups need theirs in `update-defaults`;
  `directory-grouping` needs its block. When the cooldown is enforced, `update-missing-cooldown-settings` does not
  fill in its missing fields.

The pull request lists the reset settings. When they are the only change, `pull-request-parameters` can set its own
title, commit message and a note, for example to explain why and how to request an exception:

```yaml
pull-request-parameters:
  enforce-pr-title: "[dependabutler] apply the default dependabot settings"
  enforce-commit-message: "[dependabutler] apply the default dependabot settings"
  enforce-pr-note: |
    These settings follow the defaults. To keep your own, add the repository to the exceptions file.
```

The note is added to every pull request with reset settings. The title is set only when the pull request is created:
an open dependabutler pull request keeps its title when enforced settings are added to it.

### Parameters

| parameter           | mandatory | default             | description                                   |
|---------------------|-----------|---------------------|-----------------------------------------------|
| mode                | yes       | local               | local, remote or report                       |
| configFile          | yes       | dependabutler.yml   | yml file holding the config for the tool      |
| execute             | yes       | false               | true: create PR / write file; false: log-only |
| dir                 | ¹         | *current directory* | directory containing repositories             |
| org                 | ²         |                     | organisation name on GitHub                   |
| repo                | ³         |                     | name of the repository to scan; in local mode, the name used for schedule slots |
| repoFile            | ³         |                     | file containing repositories, one per line    |
| stable-group-prefixes | no      | true                | ensures group names have numeric prefixes (01_, 02_, etc.), numbered in the order the groups are written |
| update-missing-cooldown-settings | no | true          | update existing manifests adding default settings |

¹ mandatory for local mode  
² mandatory for remote and report mode  
³ one of `repo` and `repoFile` required for remote and report mode (if both are set, `repo` takes precedence)

### GitHub API Rate Limits

GitHub enforces API rate limits (e.g. 5000 requests per hour), and each repository takes several API calls. The
remaining budget is read from the `X-RateLimit-*` headers of the API responses dependabutler already receives, so no
configuration is needed. When the budget is used up or a call is rejected, dependabutler waits until the reset time
reported by GitHub; a repository that ran into the limit halfway through is retried once after the reset.

The `GET /rate_limit` endpoint is deliberately not used: it has been observed reporting an untouched budget
(`used=0`, `remaining=5000`) with a reset sliding along with wall-clock time, while the counter enforced on the same
token's other requests had already been spent. GitHub's documentation also recommends the response headers over that
endpoint.


### Local Mode

Scan a local directory and write the `dependabot.yml` file back.

Examples:

- `dependabutler`  
  scan the current directory, log-only mode

- `dependabutler -execute=true`  
  scan the current directory and write `.github/dependabot.yml`

- `dependabutler -dir=/home/joe/myproject/ -configFile=/home/joe/dependabutler.yml -execute`  
  scan `/home/joe/myproject` and write `/home/joe/myproject/.github/dependabot.yml`, using config in `/home/joe/dependabutler.yml`


### Remote Mode
Scan a repo on GitHub using the API, and create a pull request for the `dependabot.yml` file.
For remote mode, a GitHub API token is required. It must be provided as an environment variable named `GITHUB_TOKEN`.

Examples:

- `dependabutler -mode=remote -org=acme -repo=myproject`  
  scan github.com/acme/myproject, log-only mode

- `dependabutler -mode=remote -org=acme -repo=myproject -execute=true`
  scan github.com/acme/myproject and create a PR if needed

- `dependabutler -mode=remote -org=acme -repoFile=repolist.txt -execute=true`  
  scan all projects listed in `repolist.txt` and create PRs if needed


### Report Mode
Scan repos on GitHub like remote mode, and print what dependabutler detects instead of changing anything: the manifest
files found, whether an update entry in `dependabot.yml` covers each of them, and the update entries with their
schedule. Report mode never writes: no branch, commit, pull request or label is created, and `execute` has no effect.
It needs the same parameters and `GITHUB_TOKEN` as remote mode.

Each repo is printed as one JSON object per line on stdout, in the order of the input. Log messages go to stderr.

Examples:

- `dependabutler -mode=report -org=acme -repoFile=repolist.txt 2>/dev/null | jq -c 'select(.manifests) | {repo, uncovered: [.manifests[] | select(.covered | not) | .path]}'`  
  list the manifests of each repo that no update entry covers

Output for a repo that could be read:

```json
{"repo":"myproject","default_branch":"main","dependabot_yml":true,"tree_truncated":false,"manifests":[{"path":"Dockerfile","ecosystem":"docker","covered":true},{"path":"web/package.json","ecosystem":"npm","covered":false}],"updates":[{"ecosystem":"docker","directory":"/","schedule":{"interval":"weekly","day":"sunday","timezone":"Europe/Berlin"}}]}
```

| field                 | description                                                                               |
|-----------------------|-------------------------------------------------------------------------------------------|
| repo                  | name of the repository, as given in `repo` or `repoFile`                                  |
| default_branch        | branch that was scanned                                                                   |
| dependabot_yml        | whether `.github/dependabot.yml` exists                                                   |
| tree_truncated        | GitHub returned an incomplete file list, so some manifests may be missing from the report |
| manifests             | manifest files matching `manifest-patterns`, sorted by path                               |
| manifests[].path      | path of the file, relative to the repository root                                         |
| manifests[].ecosystem | the `manifest-patterns` key the file matched                                              |
| manifests[].covered   | whether an existing update entry covers the file (the same check as in remote mode)       |
| updates               | update entries of `dependabot.yml`, in file order                                         |
| updates[].ecosystem   | `package-ecosystem` of the entry                                                          |
| updates[].directory   | `directory` of the entry, if set                                                          |
| updates[].directories | `directories` of the entry, if set                                                        |
| updates[].schedule    | `interval`, and `cronjob`, `day`, `time` and `timezone` if set                            |

Registries are not part of the report, so no registry URLs or credentials are printed.

A repo that is skipped or cannot be read is still printed, with only `repo` and one of these fields:

```json
{"repo":"oldproject","skipped":"archived"}
{"repo":"missing","error":"GET https://api.github.com/repos/acme/missing: 404 Not Found []"}
```

`skipped` is `archived` or `empty`. As in remote mode, the exit code is 1 if any repo has an `error`.

Known limits:

- Detection relies on `manifest-patterns`: files that match no pattern are not reported. To find candidates for a new
  pattern, run the report with an extended pattern in a copy of the configuration file.
- Coverage uses the same rules as remote mode: an entry covers the manifests in its directory and below, except for
  `docker`, where the directory must match exactly, and `github-actions`, where one entry for `/` covers all
  workflows. Glob patterns in `directories` (like `/apps/*`) are not recognized, so the manifests they cover are
  reported as not covered.


## Contributing

If you're interested in contributing to this project or running a dev version, have a look into the [CONTRIBUTING](CONTRIBUTING.md) document.


## Legal

Copyright 2026 GetYourGuide GmbH.

dependabutler is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text.
