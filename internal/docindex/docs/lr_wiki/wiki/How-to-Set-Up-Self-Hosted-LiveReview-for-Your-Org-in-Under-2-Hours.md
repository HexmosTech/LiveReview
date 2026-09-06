In this guide, we will walk you through the step-by-step tasks required to install and set up a self-hosted version of LiveReview on a fresh server.

## Minimum Server Requirements
* OS: Any modern operating system (Linux preferred, e.g., Ubuntu, Debian, CentOS, etc.)
* CPU: Minimum 2-core vCPU
* RAM: Minimum 2 GB to 4 GB
* Storage: Minimum 10 GB (including space for the database)

## Dependencies
* Docker and Docker Compose: LiveReview runs inside Docker containers, so we highly recommend having them installed and running on your server before starting. For Docker installation instructions, check the [official guide](https://docs.docker.com/engine/install/).
* Reverse Proxy: Ensure you have a reverse proxy configured, such as [Nginx](https://nginx.org/en/docs/install.html).
* Domain: Have a subdomain or domain ready to point to your LiveReview instance.
* Certbot: Required for setting up SSL/TLS certificates.
* Rclone: Optional, but recommended if you want to perform cloud backups (e.g., to AWS S3).


### Step 1: Run the One-Line Installer
LiveReview provides an interactive installer (`lrops.sh`) that sets up the database, Docker containers, and default configurations. Run one of the following commands in your server terminal:

**Option A: Express Mode (Recommended for quick start)**
Skips prompts and uses secure, auto-generated defaults.
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | bash -s -- --express
```

**Option B: Interactive Mode**
Guides you through optional configurations like setting custom ports and database passwords.
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | bash
```
<img width="100%" height="auto" alt="08604d9e-23ab-4764-896e-21b7050ee715" src="https://github.com/user-attachments/assets/0773b7ef-f104-4dfe-afa1-5bad35f37945" />

### Step 2: Verify the Installation
The installer automatically installs the `lrops.sh` management script to `/usr/local/bin/`. You can use this to check the health and status of your installation:

```bash
lrops.sh status
```

You can also run `lrops.sh info` to get a summary of your access URLs and configuration locations. By default, your installation will live in `$HOME/livereview/`.

<img width="100%" height="auto" alt="Screenshot_2026-06-28_19-48-01" src="https://github.com/user-attachments/assets/9af67931-6ba6-48ec-93f2-16acf4f5d131" />

### Step 3: Configure a Reverse Proxy (Recommended)
To expose LiveReview cleanly to the web (e.g., on port 80/443 without trailing ports), set up a reverse proxy.

1. The installer provides pre-made templates for Nginx, Caddy, and Apache in `/opt/livereview/config/`.
2. Run `lrops.sh help nginx` (or `caddy`/`apache`) for guidance.
3. Copy the template to your web server's configuration directory, modify it with your domain, and restart your web server.

<img width="100%" height="auto" alt="Screenshot_2026-06-28_19-51-02" src="https://github.com/user-attachments/assets/7cec4f97-36d2-426a-97d7-e71dafaeec3a" />

### Step 4: Secure with SSL/TLS (HTTPS)
For a production environment, you need HTTPS.

1. Ensure `certbot` is installed on your server.
2. Run `lrops.sh help ssl` to view template commands for generating your certificates.
3. Update your reverse proxy configuration to point to the newly generated SSL certificates and reload your proxy service.

<img width="100%" height="auto" alt="Screenshot_2026-06-28_19-53-11" src="https://github.com/user-attachments/assets/8d53d2ef-08bb-407c-94f1-43b5866ce964" />

### Step 5: Configure Automated Backups
Protect your data by setting up automated database backups.

1. Run `lrops.sh help backup` to view strategies.
2. Check `/opt/livereview/scripts/backup.sh` for a simple local backup script.
3. For cloud backups, view the cron template at `/opt/livereview/config/backup-cron.example`, which demonstrates how to use `rclone` to sync your backups to cloud storage like AWS S3 daily.

<img width="100%" height="auto" alt="Screenshot_2026-06-28_19-54-19" src="https://github.com/user-attachments/assets/396296d5-f54c-4d3c-bf2f-ebc6440f47c9" />

### Step 6: Access the Application
Once everything is running, access LiveReview via your browser:

- **Web UI:** Navigate to `https://<your-configured-domain>` (or `http://<your-server-ip>:8081` if no proxy).
- **API Base URL:** Accessible at `https://<your-configured-domain>/api` (or `http://<your-server-ip>:8888/api`).

### Step 7: Create an Organization
When you log in for the first time, you will need to set up your organization workspace.

1. Enter your organisation name.
2. Enter the super admin email and a new password to initialize your organization's dashboard.

<img width="100%" height="auto" alt="bf7de477-804e-4ea2-b5d4-4cbce997ac67" src="https://github.com/user-attachments/assets/fb2f9562-edf3-4cc5-b705-4f20fe9b1ee6" />


To activate your self-hosted deployment, you need a valid license key.

1. Click the **Have a license** option on the dashboard.
2. Click **Get a License** from the modal.

<img width="100%" height="auto" alt="0f004cd9-fcf2-4dcb-a848-c883a0b15ab5" src="https://github.com/user-attachments/assets/04532782-3532-44f7-9d9e-31647a33488c" />

3. Sign in to Hexmos.

<img width="100%" height="auto" alt="39a3bb47-7a54-4127-b90a-d1de88315267" src="https://github.com/user-attachments/assets/2626b052-e91f-4708-b00a-971122cbe14a" />

4. Click the **Regenerate Token** button.

<img width="100%" height="auto" alt="ac2bc261-6ffe-4beb-b60a-2b335b8ad852" src="https://github.com/user-attachments/assets/ace9bee6-277f-48b5-a449-0ffbc0036871" />

5. Copy the LiveReview access token.

<img width="100%" height="auto" alt="938a44d2-e0bc-4255-89a8-978bebe16653" src="https://github.com/user-attachments/assets/5b500197-b5ea-4819-9c3c-5e4da20cf3c4" />

6. Paste the token into the modal on the dashboard.

<img width="100%" height="auto" alt="8ba039e5-4698-4bb6-8702-85c8cf7a1639" src="https://github.com/user-attachments/assets/c6e20f51-a517-471b-ac56-f0a66e072660" />


### Step 9: Add an AI Provider
LiveReview requires an AI provider (like OpenAI, Anthropic, or Gemini) to generate reviews.

1. Click **AI Providers** from the Navbar.
2. Select your preferred AI provider (e.g., Gemini, Anthropic, OpenAI, OpenRouter, etc.).
3. Add the details such as the model name and API key, then click **Save Connection**.

<img width="100%" height="auto" alt="5ed8e93f-5024-449f-8138-6381ab62b8f6" src="https://github.com/user-attachments/assets/1c9a63c8-752a-40ff-b8b7-a627f154d410" />

### Step 10: Invite Team Members
Finally, invite your colleagues so they can start reviewing code.

1. Navigate to **Settings > User Management**.
2. Click **Invite User**.
3. Enter their email addresses, name, password and assign them the appropriate roles (e.g., Member, Admin).
4. Submit & Download Credentials or send an invitation if SMTP is configured.

<img width="100%" height="auto" alt="Screenshot_2026-06-28_20-07-33" src="https://github.com/user-attachments/assets/f89bff6c-15fe-48b8-a33c-eea33d84b968" />

## Conclusion
In this document, we went through the major steps for installing and setting up a self-hosted instance of LiveReview. Apart from this, you can also connect `git-lrc`, add a Git provider, configure custom prompt rules and instructions, and much more to fully customize your workflow!

Here is the complete video walkthrough for each feature: https://www.youtube.com/playlist?list=PLccZ-o1s5ANk
