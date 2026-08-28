# Microsoft Teams Integration — Local Testing

For local testing you don't need a real Azure Bot or Microsoft Teams at all — Microsoft's own **365 Agents Playground** talks directly to your local server.

## 1. Configure it in LiveReview

1. Go to **Settings → Integrations → Teams** in the LiveReview web UI.
2. Enter a **Bot App ID** and **Bot Password** — for local testing with the Playground these can be any placeholder values, they don't need to be real Azure credentials.
3. Save.

## 2. Allow local replies

The bot normally refuses to reply to anything except real Microsoft servers, as a security check. The Playground runs locally, so turn that check off for your dev session:

```bash
export TEAMS_BOT_ALLOW_LOCAL_SERVICEURL=true
```

Set this **before** starting the LiveReview server.

## 3. Restart the server

Saving the config only writes it to the database — the bot only loads it at server startup, so restart the server now (with the env var above set).

## 4. Run the Playground

No global install needed — run it straight from `npx`:

```bash
npx agentsplayground -e "http://localhost:8888/api/messages" -c "emulator"
```

- Use port `8888` (the LiveReview backend), not `8081` (the frontend).
- This opens a chat window in your browser, talking to your local bot.

## 5. Test it

1. In the Playground chat window, ask something like "how many reviews do we have?".
2. Confirm you get a reply, including charts if you ask for one.

## Troubleshooting

- **`refusing to post reply: untrusted serviceUrl`** → you skipped step 2 (the env var) or didn't restart the server after setting it.
- **Bot doesn't respond at all** → check you saved a Teams config for an org that also has a working AI connector set up.
- **Charts don't show images** → set `TEAMS_BOT_BASE_URL` to your server's address (defaults to `http://localhost:8888`) before starting the server.

## Testing with real Microsoft Teams instead

If you want the full real thing (real Azure Bot, real Teams client) instead of the Playground:

1. Create an **Azure Bot** resource in the Azure Portal, get its **App ID** and generate a **Client Secret**.
2. Run a tunnel: `ngrok http 8888`, and set the Azure Bot's messaging endpoint to `https://<your-tunnel>/api/messages`.
3. Set `TEAMS_BOT_BASE_URL` to your tunnel URL before starting the server.
4. Use the same App ID/Secret in LiveReview Settings.
5. In Azure Portal, use **Test in Web Chat** to confirm it works before touching Teams itself.
6. Add the **Microsoft Teams** channel to the bot, then sideload a Teams app package pointing at it to chat from real Teams.

You don't need `TEAMS_BOT_ALLOW_LOCAL_SERVICEURL` for this path — real Teams traffic already passes the security check.
