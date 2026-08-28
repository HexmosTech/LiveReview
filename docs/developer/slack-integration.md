# Slack Integration — Local Testing

## 1. Create the app and get your tokens

Do this from **Settings → Integrations → Slack** in the LiveReview web UI — it has a manifest ready to copy and walks you through the same steps below.

1. Go to [api.slack.com/apps](https://api.slack.com/apps) → **Create New App** → **From manifest**.
2. Paste the manifest from the LiveReview Settings page and pick your workspace.
3. Click **"Next" > "Create and Install"**.
4. When prompted to allow the app to access Slack, click **Allow**.
5. Go to **Basic Information → App-Level Tokens** → **Generate Token and Scopes** → add the scope **`connections:write`** → click **Generate**. This is your **App-Level Token**.
6. Go to **Install App** in the left sidebar → click **Install to Workspace**. This reveals your **Bot Token**.

## 2. Configure it in LiveReview

1. Paste the **App-Level Token** and **Bot Token** into the Slack section in Settings, then Save.
2. **Restart the LiveReview server.** The bot only connects at server startup, so a config saved while the server is already running won't take effect until you restart.

## 3. Test it

1. Watch the server logs for:
   ```
   [SlackBot] Org <id>: authenticated as <botname>, team=<team_id>
   ```
2. In your Slack workspace, DM the bot or `@mention` it in a channel it's in.
3. Ask something like "how many reviews do we have?" and confirm you get a reply.

## Troubleshooting

- **Config saved but bot never replies** → restart the server (step 2.2).
- **App-Level Token field rejected / socket mode won't connect** → the token needs the `connections:write` scope (step 1.5).
- **Bot Token missing/empty** → you need to actually click **Install to Workspace** (step 1.6), not just "Create and Install" earlier.
