package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-sphere/confstore"
	"github.com/go-sphere/confstore/codec"
	"github.com/go-sphere/confstore/provider"
	"github.com/go-sphere/confstore/provider/file"
	"github.com/go-sphere/confstore/provider/http"
)

type BackupProviderConfigType string

const (
	BackupProviderConfigTypeGitea BackupProviderConfigType = "gitea"
	BackupProviderConfigTypeLocal BackupProviderConfigType = "local"
)

type UnmatchedRepoAction string

const (
	UnmatchedRepoActionDelete UnmatchedRepoAction = "delete"
	UnmatchedRepoActionIgnore UnmatchedRepoAction = "ignore"
	UnmatchedRepoActionAsk    UnmatchedRepoAction = "ask"
)

type BackupProviderConfig struct {
	Type   BackupProviderConfigType `json:"type"`
	Config json.RawMessage          `json:"config"`
}

type DefaultConfig struct {
	GithubToken         string                `json:"github_token"`
	RepoOwner           string                `json:"repo_owner"`
	Backup              *BackupProviderConfig `json:"backup"`
	Filter              *FilterConfig         `json:"filter"`
	SpecificGithubToken map[string]string     `json:"specific_github_token"`
}

type GithubConfig struct {
	Owner               string                `json:"owner"`
	Token               string                `json:"token"`
	IsOwnerOrg          bool                  `json:"is_owner_org"`
	RepoOwner           string                `json:"repo_owner"`
	IsRepoOwnerOrg      bool                  `json:"is_repo_owner_org"`
	Backup              *BackupProviderConfig `json:"backup"`
	Filter              *FilterConfig         `json:"filter"`
	SpecificGithubToken map[string]string     `json:"specific_github_token"`
}

type FilterConfig struct {
	UnmatchedRepoAction UnmatchedRepoAction `json:"unmatched_repo_action"`
	PreDeleteCheckCount int                 `json:"pre_delete_check_count"`
	AllowRule           []string            `json:"allow_rule"`
	DenyRule            []string            `json:"deny_rule"`
}

func (c *GithubConfig) MergeDefault(defaultConf *DefaultConfig) {
	if defaultConf != nil {
		if c.Token == "" {
			c.Token = defaultConf.GithubToken
		}
		if c.RepoOwner == "" {
			c.RepoOwner = defaultConf.RepoOwner
		}
		if c.Backup == nil {
			c.Backup = defaultConf.Backup
		}
		if c.Filter == nil && defaultConf.Filter != nil {
			filter := *defaultConf.Filter
			filter.AllowRule = slices.Clone(filter.AllowRule)
			filter.DenyRule = slices.Clone(filter.DenyRule)
			c.Filter = &filter
		}
		if len(c.SpecificGithubToken) == 0 {
			c.SpecificGithubToken = defaultConf.SpecificGithubToken
		}
	}
	if c.RepoOwner == "" {
		c.RepoOwner = c.Owner
	}
	if c.Filter == nil {
		c.Filter = &FilterConfig{}
	}
	if defaultConf != nil && defaultConf.Filter != nil {
		if c.Filter.UnmatchedRepoAction == "" {
			c.Filter.UnmatchedRepoAction = defaultConf.Filter.UnmatchedRepoAction
			c.Filter.PreDeleteCheckCount = defaultConf.Filter.PreDeleteCheckCount
		}
		if len(c.Filter.AllowRule) == 0 {
			c.Filter.AllowRule = slices.Clone(defaultConf.Filter.AllowRule)
		}
		if len(c.Filter.DenyRule) == 0 {
			c.Filter.DenyRule = slices.Clone(defaultConf.Filter.DenyRule)
		}
	}
	if c.Filter.UnmatchedRepoAction == "" {
		c.Filter.UnmatchedRepoAction = UnmatchedRepoActionIgnore
	}
}

type SyncConfig struct {
	DefaultConf *DefaultConfig  `json:"default_conf"`
	Targets     []*GithubConfig `json:"targets"`
	Cron        string          `json:"cron"`
	StateFile   string          `json:"state_file"`
}

func Convert[T any](raw json.RawMessage) (*T, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("provider config is empty")
	}
	conf := new(T)
	err := json.Unmarshal(raw, conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func ToRaw[T any](conf T) json.RawMessage {
	raw, err := json.Marshal(conf)
	if err != nil {
		return nil
	}
	return raw
}

func NewConfig(path string) (*SyncConfig, error) {
	prov, err := provider.Selector(
		path,
		provider.If(file.IsLocalPath, func(s string) provider.Provider {
			return file.New(path, file.WithExpandEnv())
		}),
		provider.If(http.IsRemoteURL, func(s string) provider.Provider {
			return http.New(path, http.WithTimeout(10))
		}),
	)
	if err != nil {
		return nil, err
	}
	config, err := confstore.Load[SyncConfig](prov, codec.JsonCodec())
	if err != nil {
		return nil, err
	}
	if config.StateFile == "" {
		if override := os.Getenv("GITHUB_BACKUP_STATE_FILE"); override != "" {
			config.StateFile = override
		} else if file.IsLocalPath(path) {
			config.StateFile = path + ".state.json"
		} else {
			stateDir, err := os.UserConfigDir()
			if err != nil {
				return nil, fmt.Errorf("find state directory: %w", err)
			}
			hash := sha256.Sum256([]byte(path))
			config.StateFile = filepath.Join(stateDir, "github-backup", fmt.Sprintf("state-%x.json", hash[:8]))
		}
	}
	return config, nil
}
