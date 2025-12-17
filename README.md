# dependabutler

[![Go Report Card](https://goreportcard.com/badge/github.com/getyourguide/dependabutler)](https://goreportcard.com/report/github.com/getyourguide/dependabutler)

<img alt="dependabutler logo" src="dependabutler.png" style="width:48px"/>

Automatically create or update the `dependabot.yml` config file of GitHub repositories, based on manifest files present.

> `dependabutler` is a **Work In Progress** project.



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
| rateLimitBuffer                  | no⁴   | 0                   | GitHub API rate limit buffer (0=disabled). |

¹ mandatory for local mode  
² mandatory for remote mode  
³ one of `repo` and `repoFile` required for remote mode (if both are set, `repo` takes precedence)  
⁴ GitHub enforces API rate limits (e.g., 5000 requests per hour). Each repository may require multiple API calls,
depending on the number of manifest files it contains. When the remaining API calls fall below a defined threshold, 
the tool pauses and waits for the rate limit to reset. To prevent interruptions, set the buffer threshold based on 
the expected number of API calls per repository (for example, 20 calls per repo).


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


## Security

For sensitive security matters please contact [security@getyourguide.com](mailto:security@getyourguide.com).


## Legal

Copyright 2025 GetYourGuide GmbH.

dependabutler is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text.