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

### Parameters

| parameter           | mandatory | default             | description                                   |
|---------------------|-----------|---------------------|-----------------------------------------------|
| mode                | yes       | local               | local or remote                               |
| configFile          | yes       | dependabutler.yml   | yml file holding the config for the tool      |
| execute             | yes       | false               | true: create PR / write file; false: log-only |
| dir                 | ¹         | *current directory* | directory containing repositories             |
| org                 | ²         |                     | organisation name on GitHub                   |
| repo                | ³         |                     | name of the repository to scan                |
| repoFile            | ³         |                     | file containing repositories, one per line    |
| stable-group-prefixes | no      | true                | ensures group names have numeric prefixes (01_, 02_, etc.) |
| update-missing-cooldown-settings | no | true          | update existing manifests adding default settings |
| rateLimitBuffer                  | no⁴   | 0                   | GitHub API rate limit safety buffer (0=react only once the limit is hit). |

¹ mandatory for local mode  
² mandatory for remote mode  
³ one of `repo` and `repoFile` required for remote mode (if both are set, `repo` takes precedence)  
⁴ GitHub enforces API rate limits (e.g., 5000 requests per hour). Each repository may require multiple API calls,
depending on the number of manifest files it contains. The remaining budget is read from the `X-RateLimit-*` headers
of the API responses dependabutler already receives, and it waits until the reported reset time before continuing. A
repository that ran into the limit while being processed is retried once after the reset.

Set the buffer to pause *before* the limit is reached, based on the number of API calls a single repository may need
(for example, 500 for an organisation containing large monorepos). With the buffer at 0 the tool still recovers, but
only after individual calls have started to fail.

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


## Contributing

If you're interested in contributing to this project or running a dev version, have a look into the [CONTRIBUTING](CONTRIBUTING.md) document.


## Legal

Copyright 2026 GetYourGuide GmbH.

dependabutler is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text.
