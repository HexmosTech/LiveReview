# Settings → Integrations

**Route:** `/settings#integrations`
**Component:** `ui/src/pages/Settings/IntegrationsTab.tsx`
**Who sees it:** any org member; **self-hosted only** — in cloud mode the tab
renders an "Enterprise" notice saying Slack, Microsoft Teams, and Discord
integrations are not available on that plan, with no connect options.

## Purpose

Connect LiveReview's bot (**Livi**) to your team's chat platform, so people
can ask Livi questions and get review notifications where they already work.
Three integrations are offered, each configured independently on this page:
**Slack**, **Microsoft Teams**, and **Discord**.

Each one is a *bring-your-own-bot* setup: you register an app on the chat
platform yourself, then paste its credentials into this page. LiveReview does
not host a shared public bot for these.

The Slack and Discord flows offer a **Download Livi icon** button
(`/assets/lrbot/lrbot-original.png`) for use as the bot's avatar. Teams does
not - its branding comes from the uploaded app package instead.

## Slack setup

Uses Slack **Socket Mode**, so it needs two tokens: an app-level token and a
bot token.

1. In the [Slack API Apps](https://api.slack.com/apps) portal, click
   **Create New App**, choose **From manifest**.
2. Paste the manifest shown on the page (there is a **Copy** button) and
   choose your workspace. The manifest defines the bot as `Livi` and requests
   scopes including `channels:read`, `app_mentions:read`, `channels:history`,
   `chat:write`, `files:write`, `groups:history`, `im:history`, `im:read`,
   `im:write`, `users:read`, with `app_mention` / `message.im` bot events,
   interactivity, and socket mode enabled.
3. Download the Livi icon, then in Slack go to **Basic Information > Display
   Information**, upload it as the App icon, and **Save Changes**.
4. Go to **Basic Information > App-Level Tokens**, click **Generate Token and
   Scopes**, add the `connections:write` scope, **Generate**, and copy it into
   the **App-Level Token** field (format `xapp-1-...`).
5. Go to **Install App** and click **Install to Workspace**. Click **Allow**
   when prompted, to reveal the **Bot Token** (format `xoxb-...`). Copy it in.
6. Click **Save**. Both tokens are required before Save enables.

## Microsoft Teams setup

Teams needs an Azure Bot registration **and** an uploaded app package — the
Azure side alone is not enough, because Teams only lets people find and add
Livi through an installed app.

1. In the [Azure Portal](https://portal.azure.com), create an **Azure Bot**
   resource, name it **Livi**, choose **Single Tenant**. Copy the **Microsoft
   App ID** and **Directory (tenant) ID** into the **Bot App ID** and
   **Tenant ID** fields. Both must be GUIDs — the page warns if a value looks
   autofilled with something else (e.g. an email).
2. On the bot resource's **Configuration** page, set the **Messaging
   endpoint** to `<your LiveReview origin>/api/messages`.
3. Generate a **Client Secret** (**App registrations > your bot's app >
   Certificates & secrets > New client secret**). Copy the value immediately —
   Azure shows it only once — into **Bot Password (Client Secret)**.
4. Under the bot resource's **Channels**, add the **Microsoft Teams** channel.
5. Click **Save**.
6. **After saving**, return to this page and download the **Livi Teams App
   package**, then have a Teams admin upload it via the
   [Teams admin center](https://admin.teams.microsoft.com) (not the general
   Microsoft 365 admin center) — **Teams apps → Manage apps → Actions →
   Upload new app**. This is the step that actually makes Livi
   installable/usable in Teams.

## Discord setup

1. Go to the
   [Discord Developer Portal](https://discord.com/developers/applications),
   click **New Application**, name it **Livi**, then **Create**.
2. Copy the **Application ID** (from the app's **General Information** tab)
   into the Application ID field.
3. Click **Bot** in the left sidebar.
4. Download the Livi icon and upload it as the bot's avatar on the Bot page.
5. Under **Privileged Gateway Intents**, enable both and save changes:
   - `MESSAGE CONTENT INTENT` — required to read message content
   - `SERVER MEMBERS INTENT` — required to see guild members
6. Click **Reset Token**, paste the bot token into **Bot Token**, and
   click **Save**.
7. After saving, click **Invite bot to your server** (the page builds the
   OAuth2 authorize URL from your Application ID), choose your server, and
   authorize.
8. DM the bot, or mention it in a channel (`@Livi your question`), to start.

## Key actions

- View which of Slack / Teams / Discord are currently configured.
- Connect an integration by pasting its credentials, or **Edit** an existing
  one.
- **Disconnect** an integration (confirmation modal).
- Download the Livi icon (Slack/Discord), the Slack app manifest, or the
  Teams app package.
- Follow the generated invite link to add the Discord bot to a server.

## Related pages

[Settings overview](settings-overview.md), [Contact us](../contact.md),
[Git Providers](../git/git-providers.md)

## Learn more (public docs)

- [Integrations overview — bringing Livi into Slack, Discord, and Microsoft Teams](https://hexmos.com/livereview/docs/livereview/integrations)
- [Connect Livi to your Slack workspace](https://hexmos.com/livereview/docs/livereview/integrations/slack)
- [Connect Livi to Microsoft Teams via an Azure Bot](https://hexmos.com/livereview/docs/livereview/integrations/teams)
- [Connect Livi to your Discord server](https://hexmos.com/livereview/docs/livereview/integrations/discord)
