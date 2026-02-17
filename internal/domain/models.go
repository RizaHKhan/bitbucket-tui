package domain

type Project struct {
	Key             string
	Name            string
	UUID            string
	RepositoriesURL string
}

type Repository struct {
	Name       string
	Slug       string
	UUID       string
	Mainbranch string
	UpdatedOn  string
}

type Branch struct {
	Name   string
	Target BranchTarget
}

type BranchTarget struct {
	Hash string
	Date string
}

type PullRequest struct {
	ID           int
	Title        string
	Description  string
	State        string
	Draft        bool
	Author       string
	SourceBranch string
	DestBranch   string
	CreatedOn    string
	UpdatedOn    string
	URL          string
}

type Pipeline struct {
	UUID        string
	BuildNumber int
	State       string
	Result      string
	CreatedOn   string
	StartedOn   string
	CompletedOn string
}

type PipelineStep struct {
	UUID        string
	Name        string
	State       string
	Result      string
	StartedOn   string
	CompletedOn string
}
