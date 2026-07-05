package azuredevops

// identity mirrors the subset of an Azure DevOps identity/user object we need.
type identity struct {
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	ImageURL    string `json:"imageUrl"`
	ID          string `json:"id"`
}

// commitRef mirrors an Azure DevOps commit reference.
type commitRef struct {
	CommitID string `json:"commitId"`
}

type projectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type repositoryRef struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Project projectRef `json:"project"`
}

// pullRequest mirrors the subset of fields returned by the Get Pull Request API.
type pullRequest struct {
	PullRequestID         int64         `json:"pullRequestId"`
	Status                string        `json:"status"`
	CreatedBy             identity      `json:"createdBy"`
	CreationDate          string        `json:"creationDate"`
	Title                 string        `json:"title"`
	Description           string        `json:"description"`
	SourceRefName         string        `json:"sourceRefName"`
	TargetRefName         string        `json:"targetRefName"`
	MergeStatus           string        `json:"mergeStatus"`
	LastMergeSourceCommit commitRef     `json:"lastMergeSourceCommit"`
	LastMergeTargetCommit commitRef     `json:"lastMergeTargetCommit"`
	LastMergeCommit       commitRef     `json:"lastMergeCommit"`
	Repository            repositoryRef `json:"repository"`
}

// iterationsResponse wraps the List Iterations API response.
type iterationsResponse struct {
	Count int         `json:"count"`
	Value []iteration `json:"value"`
}

type iteration struct {
	ID int `json:"id"`
}

// changesResponse wraps the Get Iteration Changes API response.
type changesResponse struct {
	ChangeEntries []changeEntry `json:"changeEntries"`
}

type changeEntry struct {
	Item         changeItem `json:"item"`
	ChangeType   string     `json:"changeType"`
	OriginalPath string     `json:"originalPath"`
}

type changeItem struct {
	ObjectID         string `json:"objectId"`
	OriginalObjectID string `json:"originalObjectId"`
	Path             string `json:"path"`
	IsFolder         bool   `json:"isFolder"`
}

// projectsResponse wraps the List Projects API response.
type projectsResponse struct {
	Count int              `json:"count"`
	Value []projectSummary `json:"value"`
}

type projectSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// repositoriesResponse wraps the List Repositories API response.
type repositoriesResponse struct {
	Value []repositorySummary `json:"value"`
}

type repositorySummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
