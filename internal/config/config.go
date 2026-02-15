package config

import (
	"fmt"
	"time"
)

type Config struct {
	baseURL   string
	BasicAuth string
	Timeout   time.Duration
	Workspace string
}

func (c Config) ProjectsURL(workspace string) string {
	return fmt.Sprintf("%s/workspaces/%s/projects", c.baseURL, workspace)
}

func FromProfile(profile Profile) Config {
	return Config{
		baseURL:   "https://api.bitbucket.org/2.0",
		BasicAuth: fmt.Sprintf("Basic %s", profile.Token),
		Timeout:   20 * time.Second,
		Workspace: profile.Workspace,
	}
}
