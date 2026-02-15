package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	Name      string
	Workspace string
	Token     string
}

type ConfigFile struct {
	DefaultProfile string
	Profiles       map[string]Profile
}

// LoadConfig reads the INI config file from ~/.config/bitbucket-cli/config
func LoadConfig() (*ConfigFile, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "bitbucket-cli", "config")
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	cfg := &ConfigFile{
		Profiles: make(map[string]Profile),
	}

	scanner := bufio.NewScanner(file)
	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Parse section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}

		// Parse key-value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if currentSection == "default" {
			if key == "profile" {
				cfg.DefaultProfile = value
			}
		} else {
			// Create profile if it doesn't exist
			profile, exists := cfg.Profiles[currentSection]
			if !exists {
				profile = Profile{Name: currentSection}
			}

			switch key {
			case "workspace":
				profile.Workspace = value
			case "token":
				profile.Token = value
			}

			cfg.Profiles[currentSection] = profile
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	return cfg, nil
}

// GetProfile returns a specific profile by name
func (c *ConfigFile) GetProfile(name string) (Profile, error) {
	profile, exists := c.Profiles[name]
	if !exists {
		return Profile{}, fmt.Errorf("profile '%s' not found", name)
	}
	return profile, nil
}

// GetDefaultProfile returns the default profile if set
func (c *ConfigFile) GetDefaultProfile() (Profile, error) {
	if c.DefaultProfile == "" {
		return Profile{}, fmt.Errorf("no default profile set")
	}
	return c.GetProfile(c.DefaultProfile)
}

// ListProfiles returns a list of all profile names
func (c *ConfigFile) ListProfiles() []string {
	profiles := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		profiles = append(profiles, name)
	}
	return profiles
}
