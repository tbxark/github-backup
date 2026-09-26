# GitHub Backup

A simple tool to back up GitHub repository to gitea or other provider.

> Legacy javascript version you can find in [legacy](https://github.com/tbxark/github-backup/tree/legacy)

### Installation

#### Build from source
```bash
go install github.com/tbxark/github-backup@latest
```

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

### Project website

The [website source](pages/) is published by [the GitHub Pages workflow](.github/workflows/pages.yml) when `pages/` changes on `master`. It can also be run manually from the Actions tab. Before the first deployment, select **Settings → Pages → Build and deployment → Source → GitHub Actions** in the repository.

### Configuration

Open [the project homepage and config editor](pages/index.html) in a browser to follow the setup guide, create or import a configuration, then copy or download `config.json`. The editor uses Vue from a CDN, so it needs an internet connection when opened. It does not automatically save entered tokens.

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
        // The backup target type: gitea or local
        "type": "local",
        // Local Git configuration
        "config": {
          // Mirror repositories are stored in SAVE_DIR/REPO_OWNER/REPO
          "root": "SAVE_DIR",
          // Optional: "pull" or "fetch" keeps working-tree clones instead of bare mirrors
          // "action": "pull",
          "questions": false
        }
      },
      // Filter rules
      "filter": {
        // When the repository is not matched, the action to be taken, currently supports delete, ignore and ask
        // ask mode prompts for confirmation and skips deletion in cron mode
        "unmatched_repo_action": "ignore",
        // Number of runs an unmatched repository must survive before deletion
        "pre_delete_check_count": 2,
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
  // Optional. For a local config, defaults to config.json.state.json.
  // Keep this file across restarts when using pre_delete_check_count.
  // "state_file": "./backup-state.json",
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

The `local` provider creates bare Git mirrors over HTTPS by default. Set `backup.config.action` to `pull` or `fetch` to create and update working-tree clones instead. This also lets installations from earlier versions keep updating their existing clones. Existing bare mirrors continue to fetch even when an action is set. The provider verifies each existing repository's origin before updating it, and uses the selected GitHub token for private repositories. The container image includes Git; mount the backup root as a persistent volume when using Docker.

`pre_delete_check_count` uses `state_file` to keep deletion counts across process restarts. If omitted, the default is a sidecar file next to a local config. `GITHUB_BACKUP_STATE_FILE` can set the path when the config has no `state_file`; the supplied systemd service and Docker Compose example use persistent locations. Preserve this file across restarts.

When multiple targets use the same backup destination and owner, deletion checks run once after all targets finish. A failed source load or migration skips deletion for that destination. Repositories from an `ignore` target remain protected even if its filter excludes them. If deletion settings differ, `ask` takes precedence over `delete`, and the largest `pre_delete_check_count` applies. Overlapping `specific_github_token` patterns are checked in lexicographic order; the first match wins. In cron mode, local `questions` cannot prompt and the affected operation reports an error.

### License

**github-backup** is released under the MIT license. See [LICENSE](LICENSE) for details.
