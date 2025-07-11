module github.com/livereview

go 1.24

require (
	github.com/knadh/koanf/parsers/toml v0.1.0
	github.com/knadh/koanf/providers/confmap v0.1.0
	github.com/knadh/koanf/providers/env v0.1.0
	github.com/knadh/koanf/providers/file v0.1.0
	github.com/knadh/koanf/v2 v2.0.1
	github.com/urfave/cli/v2 v2.25.7
	// NOTE: We still keep the GitLab client for listing MRs and other operations
	// but we've implemented a custom HTTP client to work around endpoint issues
	// with direct HTTP requests to the correct plural endpoints
	gitlab.com/gitlab-org/api/client-go v0.3.0
)

require github.com/stretchr/testify v1.8.1

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	golang.org/x/sys v0.0.0-20220908164124-27713097b956 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
