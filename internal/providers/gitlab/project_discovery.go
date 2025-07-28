package gitlab

// DiscoverProjectsGitlab returns a simple list of sample GitLab projects
func DiscoverProjectsGitlab() []string {
return []string{
"gitlab-org/gitlab",
"gitlab-org/gitlab-runner", 
"gitlab-org/gitaly",
"sample-group/sample-project",
"user/my-awesome-project",
}
}
