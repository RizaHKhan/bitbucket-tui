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
}

type Branch struct {
	Name   string
	Target BranchTarget
}

type BranchTarget struct {
	Hash string
	Date string
}
