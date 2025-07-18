# Integrating with GitLab

This document provides instructions for setting up the GitLab integration with LiveReview.

## Prerequisites

Before setting up the GitLab integration:

1. Ensure your LiveReview application domain is configured in the Settings page
2. You need administrator access to create OAuth applications in GitLab

## Setting up GitLab Integration

### For GitLab.com

1. In LiveReview, navigate to **Git Providers** and select **GitLab.com**
2. Click **Continue to OAuth Setup**
3. Follow the instructions to create a GitLab OAuth application:
   - Visit [GitLab Applications](https://gitlab.com/-/user_settings/applications)
   - Create a new application with the following details:
     - **Name**: LiveReview (or any name you prefer)
     - **Redirect URI**: Your LiveReview domain URL + `/create-docs` (e.g., `https://livereview.company.com/create-docs`)
     - Uncheck the **Confidential** checkbox
     - Enable the **api** scope
   - Click **Create Application**
   - Copy the provided **Application ID**
4. Paste the Application ID into LiveReview and click **Connect**

### For Self-Hosted GitLab

1. In LiveReview, navigate to **Git Providers** and select **Self-Hosted GitLab**
2. Enter your GitLab instance URL and click **Continue to OAuth Setup**
3. Follow the instructions to create a GitLab OAuth application:
   - Visit your GitLab instance's Applications page (usually at `https://your-gitlab-instance/-/user_settings/applications`)
   - Create a new application with the following details:
     - **Name**: LiveReview (or any name you prefer)
     - **Redirect URI**: Your LiveReview domain URL + `/create-docs` (e.g., `https://livereview.company.com/create-docs`)
     - Uncheck the **Confidential** checkbox
     - Enable the **api** scope
   - Click **Create Application**
   - Copy the provided **Application ID**
4. Paste the Application ID into LiveReview and click **Connect**

## Troubleshooting

- If you encounter connection issues, verify that your domain configuration is correct
- Check that the redirect URI exactly matches what you entered in GitLab
- Ensure the API scope is enabled for your application
- For self-hosted GitLab instances, make sure the URL is correct and includes the protocol (https://)
