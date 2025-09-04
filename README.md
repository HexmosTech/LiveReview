<img src="./assets/gfx/png/logo-with-text.png" height=80 />

# Secure, Flexible & Affordable AI Code Reviewer

*LiveReview* is a self-hosted AI code reviewer that keeps your code private, adapts to your setup, and won’t blow your budget.

<br />
<p align="center">
   <img src="./assets/gfx/jpg/home_bg.jpg" alt="LiveReview dashboard screenshot" width="95%" height="95%"/>
  
</p>
<br />

<p align="center">
   <a href="#our-approach">Philosophy</a> |
   <a href="#comparisons">Comparisons</a> |
   <a href="#features">Features</a> |
   <a href="#quick-start">Quick Start</a> |
   <a href="#installation">Installation</a>
  
</p>


## Our Approach

1. **Security-First Design 🔒**
   - **Zero Cloud Dependency**: Runs entirely on your infrastructure (on-prem or private cloud) - except for update checks and license checking. 
   - **Complete Code Privacy**: Your source code and credentials never leave your environment
   - **Risk Elimination**: Recent breaches of cloud-hosted reviewers show how misconfigurations expose repositories—LiveReview eliminates this by design
   - **Full Control**: Your code stays on your servers, period
2. **Maximum Flexibility** 
   - **Multi-Platform**: Integrates with GitHub, GitLab, and Bitbucket
   - **AI Choice Freedom**: Choose Gemini, OpenAI, or self-hosted Ollama for maximum privacy
   - **Workflow Adaptation**: Adapts to your process instead of forcing change
   - **Custom Support**: Unusual setup? We'll work with you—[open an issue](https://github.com/HexmosTech/LiveReview/issues)
3. **Transparent Affordability**
   - **Straightforward Pricing**: Significantly lower than comparable hosted tools
   - **Sustainable Model**: Paid software built for long-term reliability
   - **R&D Focus**: Investment goes to product improvements, not marketing hype
   - **Maximum Value**: Every dollar funds engineering excellence

<a id="comparisons"></a>
## How is this better than... 🆚

### **vs GitHub Copilot**
- **Multi-Platform Support**: Works with GitHub, GitLab, AND Bitbucket (not just GitHub)
- **Self-Hosted Security**: Your code stays private vs cloud-hosted risk
- **AI Choice Freedom**: Pick your AI backend vs locked into one model
- **Cost Control**: You control both costs and quality

### **vs CodeRabbit**  
- **More Affordable**: Significantly lower pricing than CodeRabbit
- **Zero Cloud Risk**: Self-hosted vs recent security breaches in cloud platforms
- **Complete Control**: Your code never leaves your infrastructure
- **Attack Prevention**: Eliminates entire class of cloud-based vulnerabilities

### **vs Building Your Own**
- **Ready Out-of-Box**: Skip months of development time
- **Complex Integration Covered**: Code host APIs, webhooks, dashboards all handled
- **AI Expertise Included**: Advanced prompt engineering and review logic included  
- **Ongoing Maintenance**: No need for ongoing MR/PR handling, user management
- **Focus on Product**: Your team builds features, not infrastructure

## Features

- Integrated Dashboard - See usage statistics, impact analysis, user activity
- Git Provider - Connect as many git providers as you want - GitHub, GitLab, Bitbucket supported already.
- AI Connector - Connect your Gemini, OpenAI or Self-Hosted Ollama Keys
- Demo and Production modes - Try it out in 5 minutes, and make it production grade quickly with built-in help
- High-quality MR Summary - goes through all the changes in its full context and produces short and medium-size summaries of reviews
- Find a large number of technical issues in areas such as: unused variables, security vulnerabilities, performance issues, missing error handling, duplicated code detection, null pointer detection, data structure fit to problem, etc. Find a full list of both technical and business benefits in the [landing page](https://hexmos.com/livereview)

## Quick Start

Get LiveReview running in under 5 minutes with our simplified two-mode deployment system:

### Demo Mode (Recommended for First Time) 🚀

Perfect for development, testing, and evaluation - no configuration required!

```bash
# Quick demo setup (localhost only, no webhooks)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | sudo bash -s -- setup-demo

# Or use the express flag (same as demo mode)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | sudo bash -s -- --express
```

**Demo Mode Features:**

- **Zero configuration** - just run and go
- **Localhost only** - secure local development
- **Manual triggers** - webhooks disabled for simplicity
- **Perfect for testing** - try LiveReview without any setup
- **Easy upgrade path** - switch to production mode anytime

**Access your demo installation:**

- Web UI: http://localhost:8081/
- API: http://localhost:8888/api

### Production Mode (External Access Ready) 🌐

For teams and production deployments with reverse proxy and webhooks:

```bash
# Production setup with reverse proxy support
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- setup-production
```

**Production Mode Features:**

- **External access** - ready for reverse proxy setup
- **Webhooks enabled** - automatic code review triggers
- **SSL/TLS ready** - secure for production use
- **Auto-configuration** - webhook URLs derived automatically

**After production installation:**

1. Configure reverse proxy (nginx/caddy) - see `lrops.sh help nginx`
2. Set up SSL/TLS certificates - see `lrops.sh help ssl`
3. Point your domain to the server
4. Configure GitLab/GitHub webhooks (auto-populated URLs)

### Two-Mode Deployment System

LiveReview uses an intelligent two-mode system that automatically adapts based on your deployment:


| Feature           | Demo Mode                       | Production Mode               |
| ------------------- | --------------------------------- | ------------------------------- |
| **Access**        | localhost only                  | External via reverse proxy    |
| **Webhooks**      | Disabled (manual triggers)      | Enabled (automatic triggers)  |
| **Configuration** | Zero config required            | Reverse proxy setup needed    |
| **Perfect for**   | Development, testing, demos     | Teams, production deployments |
| **Upgrade**       | `LIVEREVIEW_REVERSE_PROXY=true` | Ready out of the box          |

### Switching Between Modes

**Demo → Production:**

```bash
# Edit your .env file
echo "LIVEREVIEW_REVERSE_PROXY=true" >> /opt/livereview/.env

# Restart services
lrops.sh restart

# Configure reverse proxy
lrops.sh help nginx  # or caddy, apache
```

**Production → Demo:**

```bash
# Edit your .env file  
sed -i 's/LIVEREVIEW_REVERSE_PROXY=true/LIVEREVIEW_REVERSE_PROXY=false/' /opt/livereview/.env

# Restart services
lrops.sh restart
```

This will:

1. Install LiveReview with Docker and PostgreSQL
2. Auto-detect deployment mode (demo/production)
3. Set up secure defaults and auto-generated passwords
4. Deploy to `/opt/livereview/` with persistent data storage
5. Configure environment-aware webhook URLs
6. Provide access URLs and mode-specific guidance

After installation, configure your GitLab/GitHub providers, and start reviewing code!

## Installation

### Simplified Two-Mode Installer (Recommended)

LiveReview now features a two-mode deployment system that adapts to your needs:

#### Quick Demo Setup (Perfect for First Time)

Zero configuration required - perfect for development and evaluation:

```bash
# Quick demo mode (localhost only, no webhooks)
lrops.sh setup-demo

# Alternative: one-line installer
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- setup-demo
```

#### Production Setup (Teams & Production)

External access ready with reverse proxy and webhook support:

```bash
# Production mode (reverse proxy + webhooks)
lrops.sh setup-production  

# Alternative: one-line installer
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- setup-production
```

#### Legacy Express Mode (Demo Mode)

```bash
# Express mode (same as demo mode for backward compatibility)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --express

# Interactive installation (guided setup)  
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash

# Install specific version
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --version=v1.2.3 setup-demo
```

#### What the installer does:

- **Intelligent mode detection** - automatically configures demo vs production
- **System prerequisites check** (Docker, Docker Compose)
- **Smart environment configuration** - uses LIVEREVIEW_* environment variables
- **Secure password generation** - strong defaults for database and JWT
- **Docker deployment** - PostgreSQL database included
- **Webhook auto-configuration** - URLs derived based on deployment mode
- **Management script installation** (`lrops.sh`) for ongoing operations
- **Mode-specific guidance** - next steps tailored to your deployment

**After installation**, manage LiveReview with:

```bash
lrops.sh status          # Check installation status and deployment mode
lrops.sh start           # Start services  
lrops.sh stop            # Stop services
lrops.sh restart         # Restart services (useful after mode changes)
lrops.sh logs            # View container logs
lrops.sh help ssl        # SSL setup guidance (production mode)
lrops.sh help nginx      # Nginx reverse proxy setup
lrops.sh help caddy      # Caddy reverse proxy setup (automatic SSL)
lrops.sh help backup     # Backup strategies
```

#### Environment Variables (Advanced)

The two-mode system uses these environment variables for configuration:

```bash
# Core deployment configuration
LIVEREVIEW_BACKEND_PORT=8888              # Backend API port
LIVEREVIEW_FRONTEND_PORT=8081             # Frontend UI port  
LIVEREVIEW_REVERSE_PROXY=false            # Demo mode (true = production mode)

# Database and security (auto-generated)
DB_PASSWORD=<secure-generated-password>   # PostgreSQL password
JWT_SECRET=<secure-generated-secret>      # JWT signing key
```

**Mode Detection Logic:**

- `LIVEREVIEW_REVERSE_PROXY=false` → **Demo Mode** (localhost, no webhooks)
- `LIVEREVIEW_REVERSE_PROXY=true` → **Production Mode** (reverse proxy, webhooks enabled)


#### Multi-Architecture Support

LiveReview Docker images support multiple architectures to resolve GitLab Container Registry "blob unknown" errors when creating manifest lists:

- **amd64**: For x86_64 systems
- **arm64**: For ARM64 systems (Apple Silicon, AWS Graviton, etc.)

The build process automatically:

1. Builds architecture-specific images with proper tags (e.g., `myapp:1.0.0-amd64`, `myapp:1.0.0-arm64`)
2. Pushes individual architecture images to the same repository
3. Creates and pushes a manifest list that references all architectures
4. Tags the manifest list with the main version tag (e.g., `myapp:1.0.0`, `myapp:latest`)

This approach ensures compatibility with GitLab Container Registry requirements for multi-architecture images.


## Usage

TODO

## License

> [!NOTE]
>
> LiveReview is a proprietary developer tool by Hexmos, built to streamline code review and help teams ship faster.
>
> Guides, documentation, roadmaps, and community discussions are fully open, making it easy to get started, provide feedback, and stay informed about product evolution.
