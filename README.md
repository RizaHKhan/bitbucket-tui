# Bitbucket TUI

This is a Bitbucket TUI which interacts with the Bitbucket the [REST API](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#authentication)

## Basline

It seems the `workspace` is a baseline requirements for all API endpoints. 

In order for this application to work, we would need to:

1. Select a `workspace`

In order to do this, we need to add credentials for each workspace (there are other methods but for simplicity lets just use API TOKENS which you can create from Atlassian).

### Configuration

# TODO: This should be ~/.config/bitbucket-tui/
Workspaces are configured in `~/.config/bitbucket-cli/config` using INI format (similar to AWS CLI):

```ini
[default]
profile = camcloud

[camcloud]
workspace = camcloud
token = YOUR_API_TOKEN_HERE

[other-workspace]
workspace = acme-corp
token = ANOTHER_API_TOKEN
```

**Fields:**
- `[default]` section: Specifies which profile to use automatically
- `[profile-name]` sections: Each workspace configuration
  - `workspace`: The Bitbucket workspace name
  - `token`: API token (Base64 encoded username:token from Atlassian)

**Security:** The config file should have permissions `600` (readable/writable by owner only):
```bash
chmod 600 ~/.config/bitbucket-cli/config
```

If no `[default]` is set, you'll need to select a workspace when the application starts.

## Adding a Go package dependency

This project uses Go modules (`go.mod` / `go.sum`).

1. Import the package in your Go file.

```go
import "github.com/charmbracelet/bubbles/spinner"
```

2. Add the dependency from the repo root:

```bash
go get github.com/charmbracelet/bubbles/spinner
```

3. Sync module files:

```bash
go mod tidy
```

4. Verify everything builds/tests:

```bash
go test ./...
```

### Useful variants

- Install latest: `go get github.com/google/uuid`
- Pin version: `go get github.com/google/uuid@v1.6.0`
- Remove dependency: `go get github.com/google/uuid@none && go mod tidy`

Always commit both `go.mod` and `go.sum` when dependencies change.
