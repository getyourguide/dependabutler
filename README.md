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
ecosystem and directory. Comments are not kept.

### Schedule slots
With `schedule-slots` in the configuration file, new update entries do not get the `schedule` of `update-defaults`.
Each repository gets its own slot instead, so that Dependabot does not run for all repositories at the same time:

```yaml
schedule:
  interval: cron
  cronjob: "0 4 * * 2,5"
  timezone: Europe/Berlin
```

A repository runs `runs-per-week` times a week (2 by default) at the same hour, on days from `days` (Monday to Friday
by default), spread over them as evenly as they divide. With the defaults, the pairs are Monday and Thursday, Tuesday
and Friday, Wednesday and Monday, Thursday and Tuesday, Friday and Wednesday. Every day is used equally often, so every
day gets about the same number of repositories.

```yaml
schedule-slots:
  salt: "01J9Z3K4M5N6P7Q8R9S0T1V2W3"
  windows:
    - name: daytime
      hours: [10, 11, 13, 14, 15]
      timezone: Europe/Berlin
      rulesets: [critical-services]
      repos-file: daytime-repos.txt
    - name: night
      hours: [2, 3, 4, 5, 6, 7, 8]
      days: [monday, tuesday, wednesday, thursday, friday]  # the default
      runs-per-week: 2                                      # the default
      timezone: Europe/Berlin
```

- A repository uses the first window that selects it: an active ruleset in `rulesets` applies to it, or `repos-file`
  lists it. The last window has neither and holds every other repository.
- Use `rulesets` to reuse a grouping the organization already has: a new repository is picked up as soon as the ruleset
  applies to it. Rulesets of the repository itself count too, so a repository admin can put their repository in a
  window by creating an active ruleset with that name. Use `repos-file` for a list kept by hand.
- `repos-file` is relative to the configuration file and lists one repository name per line. Blank lines are skipped.
- Rulesets are read with one API call per repository, only when they can change its window: not for a repository that a
  `repos-file` already places before them. If they cannot be read, the repository is skipped and counted as failed,
  rather than placed in the wrong window. In local mode rulesets are not read, and the repository name is `repo`, or the
  name of `dir` if `repo` is not set.
- Each repository gets a fixed slot, a day and an hour of its window, based on its name and `salt`. Adding or removing
  repositories does not move the others; changing the salt reshuffles all of them. Names match regardless of case, in
  `repos-file` too. The spread is not perfectly even: to reduce the busiest slot, try a few salts against the real
  repository names once, and keep the best.
- `days` are weekday names in lower case. `runs-per-week` is at most the number of days; 0 or unset means 2.
- Unknown keys under `schedule-slots` are rejected, so a typo cannot quietly put a repository in another window.
- Only new update entries get a slot. A schedule set in `update-overrides` is ignored for them, with a warning.
  Existing entries keep their schedule.

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
| stable-group-prefixes | no      | true                | ensures group names have numeric prefixes (01_, 02_, etc.) |
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
