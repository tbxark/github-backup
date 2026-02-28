# GitHub Backup

A simple tool to back up GitHub repository to gitea or other provider.

> Legacy javascript version you can find in [legacy](https://github.com/tbxark/github-backup/tree/legacy)

### Installation

#### Build from source
```bash
go install github.com/tbxark/github-backup@latest
````

### Usage
```
Usage of github-backup:
  -config string
        config file (default "config.json")
  -help
        show help
  -version
        show version

```

```bash
github-backup -config config.json
```


### Configuration

> The configuration file is a json file, the default configuration is as follows, You need to replace the placeholder with your own configuration, And delete the comments
```json5
{
  // Target configuration, will be used to backup the repository
  "targets": [
    {
      // The user or organization to be backed up
      "owner": "GITHUB_OWNER",
      // The token of the repository to be backed up
      "token": "GITHUB_TOKEN",
      // The backup target owner
      "repo_owner": "BACKUP_TARGET_REPO_OWNER",
      "backup": {
        // The backup target type, currently only supports gitea and file
        "type": "file",
        // File configuration, only used when the backup target is file
        "config": {
          // The directory where the backup file is stored
          "dir": "SAVE_DIR",
          // The history json file path
          "history": "FILE_HISTORY_JSON_PATH",
          // When debug is true, files will not be downloaded
          "debug": false
        }
      },
      // Filter rules
      "filter": {
        // When the repository is not matched, the action to be taken, currently only supports delete and ignore
        "unmatched_repo_action": "ignore",
        // Allow rules, only repositories that match the rules will be backed up
        // The rule is a regular expression, the format is :owner/:repo/:private/:fork/:archived
        // For example, the rule [a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/0/[01]/[01] means that only public repositories will be backed up
        "allow_rule": ["[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/0/[01]/[01]"],
        // Deny rules, repositories that match the rules will not be backed up
        "deny_rule": ["[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/1/[01]/[01]"]
      },
        // The specific token configuration, the key is the rule, and the value is the token
      "specific_github_token": {
        "[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/0/[01]/[01]": "PUBLIC_GITHUB_TOKEN", 
        "[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+/1/[01]/[01]": "PRIVATE_GITHUB_TOKEN"
      }
    },
    {
      // The organization to be backed up
      "owner": "GITHUB_ORG",
      // Set is_owner_org to true when the owner is an organization
      "is_owner_org": true,
      // The backup target organization 
      "repo_owner": "BACKUP_TARGET_REPO_ORG",
      // Set is_repo_owner_org to true when the backup target is an organization
      "is_repo_owner_org": true,
    }
  ],
  // Default configuration, will be used if the target configuration is not sets
  "default_conf": {
    "github_token": "YOUR_GITHUB_TOKEN",
    "repo_owner": "BACKUP_TARGET_REPO_OWNER",
    "backup": {
      "type": "gitea",
      // Gitea configuration, only used when the backup target is gitea
      "config": {
        // Gitea host, You can use your own gitea server
        "host": "GITEA_HOST",
        // Gitea token, You can create a new token in the gitea settings
        "token": "GITEA_TOKEN",
        // Gitea username, You can use your own gitea username
        "auth_username": "GITEA_USERNAME"
      }
    },
    "filter": {
      "unmatched_repo_action": "delete",
      "allow_rule": [],
      "deny_rule": []
    }
  }
}
```

### License

**github-backup** is released under the MIT license. See [LICENSE](LICENSE) for details.
