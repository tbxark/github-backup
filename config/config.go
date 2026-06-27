package config

import (
	"encoding/json"

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
	if defaultConf == nil {
		return
	}
	if c.Token == "" {
		c.Token = defaultConf.GithubToken
	}
	if c.RepoOwner == "" {
		c.RepoOwner = defaultConf.RepoOwner
	}
	if c.RepoOwner == "" {
		c.RepoOwner = c.Owner
	}
	if c.Backup == nil {
		c.Backup = defaultConf.Backup
	}
	if c.Filter == nil {
		c.Filter = defaultConf.Filter
		if c.Filter == nil {
			c.Filter = &FilterConfig{}
		}
	}
	if c.Filter.UnmatchedRepoAction == "" {
		c.Filter.UnmatchedRepoAction = defaultConf.Filter.UnmatchedRepoAction
		c.Filter.PreDeleteCheckCount = defaultConf.Filter.PreDeleteCheckCount
		if c.Filter.UnmatchedRepoAction == "" {
			c.Filter.UnmatchedRepoAction = UnmatchedRepoActionIgnore
		}
	}
	if len(c.Filter.AllowRule) == 0 {
		c.Filter.AllowRule = defaultConf.Filter.AllowRule
	}
	if len(c.Filter.DenyRule) == 0 {
		c.Filter.DenyRule = defaultConf.Filter.DenyRule
	}
	if len(c.SpecificGithubToken) == 0 {
		c.SpecificGithubToken = defaultConf.SpecificGithubToken
	}
}

type SyncConfig struct {
	DefaultConf *DefaultConfig  `json:"default_conf"`
	Targets     []*GithubConfig `json:"targets"`
	Cron        string          `json:"cron"`
}

func Convert[T any](raw json.RawMessage) (*T, error) {
	conf := new(T)
	err := json.Unmarshal(raw, &conf)
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
	return config, nil
}
