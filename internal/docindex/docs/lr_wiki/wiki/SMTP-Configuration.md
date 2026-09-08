Configure LiveReview to send email (invitations, verification, license/quota notices) through your own SMTP provider.

## 1. Where to configure it

Self-hosted only: **Settings → SMTP** (`/settings#smtp`), visible to organization Owners.

| Field | Value |
|---|---|
| Host | Your provider's SMTP hostname, e.g. `smtp.gmail.com`, `sandbox.smtp.mailtrap.io` |
| Port | `587` (STARTTLS) is the usual default — see [Troubleshooting](#troubleshooting) if it's blocked |
| Username | Provider-issued SMTP username |
| Password | Provider-issued SMTP password/API key |
| Sender Email (From) | Should match the authenticated account for providers that enforce it |
| Skip TLS Verification | Leave **unchecked** unless your server uses a self-signed certificate |

Click **Test Connection** (or **Send Test Email**) to confirm before saving.

## Troubleshooting

### SMTP not working on DigitalOcean servers

DigitalOcean blocks outbound SMTP ports **25, 465, and 587** by default on all new droplets/accounts, as an anti-spam measure. This is enforced at the network level (upstream of the droplet) — you won't see it in `ufw` or `iptables`, and it affects every SMTP provider the same way (Mailtrap, Gmail, SendGrid, Ethereal, etc.), not just one.

Symptom: `SMTP connection failed: failed to dial SMTP server: dial tcp <ip>:587: i/o timeout` even though the host/port/credentials are correct.

To confirm it's this and not a config mistake, test connectivity from the droplet itself:
```
docker exec -it livereview-app curl -v --connect-timeout 8 smtp://<your-smtp-host>:587
```
A hang followed by `Connection timed out` confirms the block.

**Fix options:**

- **Ask DigitalOcean to lift the block**: go to [DigitalOcean Support](https://do.co/support/) and explain your use case (e.g. transactional email for a self-hosted app). Sometimes they'll lift the restriction after reviewing your account. See DigitalOcean's own writeup: [Why is SMTP blocked?](https://docs.digitalocean.com/support/why-is-smtp-blocked/)
- **Use an alternate SMTP port**: most providers (Mailtrap, Mailjet, SendGrid, etc.) also support port **2525**, which is usually open on DigitalOcean and works exactly like 587 for STARTTLS. Just change the SMTP Port field to `2525` — no other config change needed.

## Related

- [Add Your Team to LiveReview](Add-Your-Team-to-LiveReview)
- [Productionize LiveReview](Productionize-LiveReview)
