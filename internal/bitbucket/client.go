package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sort"

	"bitbucket-cli/internal/config"
	"bitbucket-cli/internal/domain"
)

type Client struct {
	httpClient *http.Client
	config     config.Config
}

type projectsResponse struct {
	Values []apiProject `json:"values"`
}

type apiProject struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	UUID  string `json:"uuid"`
	Links struct {
		Repositories struct {
			Href string `json:"href"`
		} `json:"repositories"`
	} `json:"links"`
}

type repositoriesResponse struct {
	Values []apiRepository `json:"values"`
	Next   string          `json:"next"`
}

type apiRepository struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	UUID       string `json:"uuid"`
	UpdatedOn  string `json:"updated_on"`
	Mainbranch struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
}

type branchesResponse struct {
	Values []apiBranch `json:"values"`
	Next   string      `json:"next"`
}

type apiBranch struct {
	Name   string `json:"name"`
	Target struct {
		Hash string `json:"hash"`
		Date string `json:"date"`
	} `json:"target"`
}

type pullRequestsResponse struct {
	Values []apiPullRequest `json:"values"`
	Next   string           `json:"next"`
}

type apiPullRequest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	Draft       bool   `json:"draft"`
	Author      struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"destination"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
	Links     struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type pipelinesResponse struct {
	Values []apiPipeline `json:"values"`
	Next   string        `json:"next"`
}

type apiPipeline struct {
	UUID        string `json:"uuid"`
	BuildNumber int    `json:"build_number"`
	CreatedOn   string `json:"created_on"`
	CompletedOn string `json:"completed_on"`
	State       struct {
		Name  string `json:"name"`
		Stage struct {
			Name      string `json:"name"`
			StartedOn string `json:"started_on"`
		} `json:"stage"`
		Result struct {
			Name string `json:"name"`
		} `json:"result"`
	} `json:"state"`
}

type pipelineStepsResponse struct {
	Values []apiPipelineStep `json:"values"`
}

type apiPipelineStep struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	StartedOn   string `json:"started_on"`
	CompletedOn string `json:"completed_on"`
	State       struct {
		Name   string `json:"name"`
		Result struct {
			Name string `json:"name"`
		} `json:"result"`
	} `json:"state"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		config:     cfg,
	}
}

func (c *Client) ListProjects() (string, []domain.Project, error) {
	url := c.config.ProjectsURL(c.config.Workspace)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", nil, err
	}

	setJSONHeaders(req, c.config.BasicAuth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("request failed for URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.Status, nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Status, nil, fmt.Errorf("non-success status code: %d for URL %s, response: %s", resp.StatusCode, url, string(body))
	}

	var decoded projectsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return resp.Status, nil, fmt.Errorf("unable to decode projects response: %w", err)
	}

	projects := make([]domain.Project, 0, len(decoded.Values))
	for _, item := range decoded.Values {
		projects = append(projects, domain.Project{
			Key:             item.Key,
			Name:            item.Name,
			UUID:            item.UUID,
			RepositoriesURL: item.Links.Repositories.Href,
		})
	}

	return resp.Status, projects, nil
}

func (c *Client) ListRepositories() ([]domain.Repository, error) {
	var allRepos []domain.Repository
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s?pagelen=100", c.config.Workspace)

	for url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		setJSONHeaders(req, c.config.BasicAuth)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
		}

		var decoded repositoriesResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("unable to decode repositories response: %w", err)
		}

		for _, item := range decoded.Values {
			allRepos = append(allRepos, domain.Repository{
				Name:       item.Name,
				Slug:       item.Slug,
				UUID:       item.UUID,
				Mainbranch: item.Mainbranch.Name,
				UpdatedOn:  item.UpdatedOn,
			})
		}

		url = decoded.Next
	}

	sortByUpdatedOn(allRepos)

	return allRepos, nil
}

func (c *Client) ListBranches(repoSlug string) ([]domain.Branch, error) {
	var allBranches []domain.Branch
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/refs/branches?pagelen=100", c.config.Workspace, repoSlug)

	for url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		setJSONHeaders(req, c.config.BasicAuth)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
		}

		var decoded branchesResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("unable to decode branches response: %w", err)
		}

		for _, item := range decoded.Values {
			allBranches = append(allBranches, domain.Branch{
				Name: item.Name,
				Target: domain.BranchTarget{
					Hash: item.Target.Hash,
					Date: item.Target.Date,
				},
			})
		}

		url = decoded.Next
	}

	return allBranches, nil
}

func (c *Client) ListPullRequests(repoSlug string) ([]domain.PullRequest, error) {
	var allPRs []domain.PullRequest
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests?pagelen=50", c.config.Workspace, repoSlug)

	for url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		setJSONHeaders(req, c.config.BasicAuth)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
		}

		var decoded pullRequestsResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("unable to decode pull requests response: %w", err)
		}

		for _, item := range decoded.Values {
			prURL := item.Links.HTML.Href
			if prURL == "" {
				prURL = item.Links.Self.Href
			}

			allPRs = append(allPRs, domain.PullRequest{
				ID:           item.ID,
				Title:        item.Title,
				Description:  item.Description,
				State:        item.State,
				Draft:        item.Draft,
				Author:       item.Author.DisplayName,
				SourceBranch: item.Source.Branch.Name,
				DestBranch:   item.Destination.Branch.Name,
				CreatedOn:    item.CreatedOn,
				UpdatedOn:    item.UpdatedOn,
				URL:          prURL,
			})
		}

		url = decoded.Next
	}

	return allPRs, nil
}

func (c *Client) ListPipelines(repoSlug string) ([]domain.Pipeline, error) {
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pipelines?sort=-created_on&pagelen=30", c.config.Workspace, repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	setJSONHeaders(req, c.config.BasicAuth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var decoded pipelinesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("unable to decode pipelines response: %w", err)
	}

	pipelines := make([]domain.Pipeline, 0, len(decoded.Values))
	for _, item := range decoded.Values {
		pipelines = append(pipelines, domain.Pipeline{
			UUID:        item.UUID,
			BuildNumber: item.BuildNumber,
			State:       item.State.Name,
			Result:      item.State.Result.Name,
			CreatedOn:   item.CreatedOn,
			StartedOn:   item.State.Stage.StartedOn,
			CompletedOn: item.CompletedOn,
		})
	}

	return pipelines, nil
}

func (c *Client) ListPipelineSteps(repoSlug, pipelineUUID string) ([]domain.PipelineStep, error) {
	escapedUUID := neturl.PathEscape(pipelineUUID)
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pipelines/%s/steps", c.config.Workspace, repoSlug, escapedUUID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	setJSONHeaders(req, c.config.BasicAuth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var decoded pipelineStepsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("unable to decode pipeline steps response: %w", err)
	}

	steps := make([]domain.PipelineStep, 0, len(decoded.Values))
	for _, item := range decoded.Values {
		steps = append(steps, domain.PipelineStep{
			UUID:        item.UUID,
			Name:        item.Name,
			State:       item.State.Name,
			Result:      item.State.Result.Name,
			StartedOn:   item.StartedOn,
			CompletedOn: item.CompletedOn,
		})
	}

	return steps, nil
}

func (c *Client) GetPipelineStepLog(repoSlug, pipelineUUID, stepUUID string) (string, error) {
	escapedPipelineUUID := neturl.PathEscape(pipelineUUID)
	escapedStepUUID := neturl.PathEscape(stepUUID)
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pipelines/%s/steps/%s/log", c.config.Workspace, repoSlug, escapedPipelineUUID, escapedStepUUID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", c.config.BasicAuth)
	req.Header.Set("Accept", "*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("non-success status code: %d, response: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

func sortByUpdatedOn(repos []domain.Repository) {
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].UpdatedOn > repos[j].UpdatedOn
	})
}

func setJSONHeaders(req *http.Request, authValue string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authValue)
}
