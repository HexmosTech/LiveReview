# LiveReview

AI-powered code review tool for GitLab, GitHub, and BitBucket.

## Overview

LiveReview helps automate code reviews using AI across different code hosting platforms. The first implementation supports GitLab with Gemini AI integration.

## Features

- Integration with self-hosted GitLab instances
- AI-powered code review using Google's Gemini
- Command-line interface for easy use
- Configurable through TOML configuration file

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/livereview.git

# Build the binary
cd livereview
go build

# Run the binary
./livereview --help
```

## Configuration

LiveReview uses a TOML configuration file (`livereview.toml`) to store settings.

### Initialize Configuration

```bash
./livereview config init
```

This will create a `livereview.toml` file with default settings that you can customize.

### GitLab Configuration

To use LiveReview with GitLab, you need to configure your GitLab instance and provide an access token.

#### Getting a GitLab Access Token

1. **Log in to your GitLab instance** (e.g., https://git.apps.hexmos.com)

2. **Access your User Settings**:
   - Click on your avatar in the top-right corner
   - Select "Preferences" or "Settings" from the dropdown menu

3. **Navigate to Access Tokens**:
   - In the left sidebar, find and click on "Access Tokens"
   - This may be under a section called "User Settings" or directly in the sidebar

4. **Create a new token**:
   - Enter a name for your token (e.g., "LiveReview Tool")
   - Set an optional expiration date (or leave blank for a non-expiring token)
   - Select the required scopes:
     - `api` (Required for API access)
     - `read_repository` (To read repository content)
     - `write_discussion` (To post comments)

5. **Generate the token**:
   - Click the "Create personal access token" button
   - **Important**: Copy the token immediately! GitLab will only show it once.

6. **Update your configuration**:
   - Open your `livereview.toml` file
   - Replace `"your-gitlab-token"` with the actual token you just copied:

```toml
[providers.gitlab]
url = "https://git.apps.hexmos.com"
token = "glpat-xxxxxxxxxxxxxxxxx"  # Your actual token here
```

#### Alternative: Project Access Token

If you prefer to limit the token's access to specific projects:

1. Go to the project settings (Project → Settings → Access Tokens)
2. Create a project access token with appropriate permissions
3. Use this token in your configuration

#### Security Best Practices

1. **Don't commit your token**: Make sure not to commit your actual token to version control
2. **Use environment variables**: For production use, consider using environment variables:
   ```
   export LIVEREVIEW_PROVIDERS_GITLAB_TOKEN="your-token-here"
   ```
3. **Set an expiration date**: For better security, set an expiration date on your token
4. **Use minimum required scopes**: Only select the scopes you actually need

### Gemini AI Configuration

To use the Gemini AI backend, you need to provide an API key:

1. Visit [Google AI Studio](https://makersuite.google.com/) to get an API key
2. Add the key to your configuration:

```toml
[ai.gemini]
api_key = "your-gemini-api-key"  # Your actual API key here
model = "gemini-pro"
temperature = 0.2
```

## Usage

### Reviewing a Merge Request

```bash
./livereview review https://gitlab.example.com/group/project/-/merge_requests/123
```

### Options

- `--dry-run, -d`: Run review without posting comments to GitLab
- `--verbose, -v`: Enable verbose output
- `--provider, -p`: Override the default provider
- `--ai, -a`: Override the default AI backend
- `--config, -c`: Specify a different configuration file

## Architecture

LiveReview uses a modular architecture that supports multiple code hosting providers and AI backends through abstraction interfaces.

## License

[MIT License](LICENSE)
