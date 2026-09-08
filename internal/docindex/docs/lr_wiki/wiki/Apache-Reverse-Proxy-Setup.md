Put Apache in front of a self-hosted LiveReview install, secure it with SSL via `lrops.sh`, and diagnose the issues most commonly hit along the way.

## 1. Install Apache and the required modules

```
sudo apt update && sudo apt install -y apache2
sudo a2enmod proxy proxy_http headers deflate ssl
```

Each module's job:

| Module | Purpose |
|---|---|
| `proxy` | Core reverse-proxy support |
| `proxy_http` | Proxying to `http://` backends |
| `headers` | Setting security headers and forwarded-request headers |
| `deflate` | Compressing API (JSON) responses |
| `ssl` | HTTPS support (needed once you set up SSL) |

## 2. Copy and enable the LiveReview vhost

`lrops.sh` already extracted a ready-made Apache config during install, at `$LIVEREVIEW_INSTALL_DIR/config/apache.conf.example`. It correctly splits traffic between the frontend and backend, sets security headers, upload limits, and long timeouts for reviews — use it instead of writing one by hand.

```
sudo cp ~/livereview/config/apache.conf.example /etc/apache2/sites-available/livereview.conf
sudo a2ensite livereview
sudo a2dissite 000-default.conf
sudo apache2ctl configtest
sudo systemctl restart apache2
```

> **Don't skip `a2dissite 000-default.conf`.** Apache's default site otherwise wins over `livereview.conf` for any request that doesn't match a specific `ServerName` — including plain IP-address access. Skipping this step is why you'd see Ubuntu's "Apache2 Default Page" instead of LiveReview.

## 3. If you have a real domain

Substitute it into the copied vhost file:

```
sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/apache2/sites-available/livereview.conf
sudo systemctl reload apache2
```

If you're testing with a bare IP address (no domain), skip this step entirely — Apache only needs `ServerName` to choose between *multiple* vhosts sharing a port. With just one vhost active, it's used for every request regardless of what `ServerName` says.

## 4. Switch LiveReview to production mode

This matters even for a working reverse proxy: in demo mode, LiveReview hardcodes its API URL to `http://localhost:<port>`, which only works when the browser and server are the same machine. Behind Apache, from any other machine, that breaks all API calls even though the page loads.

```
lrops.sh set-mode production
```

## Firewall

Apache being configured correctly doesn't help if the firewall blocks the ports it listens on. This is the single most common cause of "the site won't load."

```
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp   # only needed once you set up SSL
sudo ufw status
```

> If you're on a cloud provider (DigitalOcean, AWS, etc.), also check for a separate **cloud firewall** attached to the server — those exist independently of `ufw` and block traffic before it even reaches the box.

## 5. SSL setup (once you have a domain)

`lrops.sh` automates certificate issuance and vhost reconfiguration end-to-end via `setup-ssl`.

**Before running it:**
- Your domain's DNS **A record** must already point at this server's IP — Let's Encrypt verifies ownership by reaching the server at that domain.
- Port `443` must be open (see [Firewall](#firewall) above).

**Run it:**

```
sudo lrops.sh setup-ssl yourdomain.com you@example.com
```

You'll be prompted to choose which reverse proxy is in use — select **Apache**. `lrops.sh` then:

1. Obtains a certificate via `certbot certonly --standalone` (briefly stopping Apache to bind port 80 for validation)
2. Sets up automatic renewal via cron
3. Copies a fresh `apache.conf.example` into place, substitutes your domain, and uncomments the `:443` HTTPS block
4. Enables the needed modules and reloads Apache
5. Verifies HTTPS actually answers, both locally and publicly

> **The email argument isn't optional in practice** — without it, `certbot` prompts interactively for one, and that prompt doesn't handle being skipped or cancelled cleanly. Always pass an email.

Once complete, `https://yourdomain.com` should be live.

## Troubleshooting

### Site doesn't load at all (times out / hangs)

Almost always a firewall issue, not an Apache configuration problem. Check, in order:

```
curl -I http://localhost/          # 1. Does Apache serve it locally?
sudo ss -tlnp | grep :80           # 2. Is Apache listening on 0.0.0.0, not just 127.0.0.1?
sudo ufw status                    # 3. Is port 80 actually allowed?
```

If step 1 and 2 look fine but the site is still unreachable from outside, it's the firewall (`ufw`, or a cloud provider's separate firewall) — see [Firewall](#firewall).

For a one-shot diagnostic covering all of this at once:

```
lrops.sh doctor
```

### Shows the Ubuntu "Apache2 Default Page" instead of LiveReview

The default site is still enabled and winning over `livereview.conf`. Fix:

```
sudo a2dissite 000-default.conf
sudo systemctl reload apache2
```

Confirm only your site is active:

```
sudo apache2ctl -S
ls /etc/apache2/sites-enabled/
```

### `apache2ctl configtest` fails with a syntax error

Two known traps in hand-edited or older Apache configs:

- **`LimitRequestBody 104857600  # 100MB`** — Apache doesn't support a trailing `# comment` on a directive line; everything after the value is parsed as extra arguments. Put the comment on its own line above instead.
- **`ProxySetHeader ...`** — not a real Apache directive (it doesn't exist; `mod_headers`'s actual directive is `RequestHeader set`). If you see "Invalid command 'ProxySetHeader'", that's the cause.

Both are already fixed in the current `apache.conf.example` template — re-copy it fresh if your install predates the fix:

```
sudo cp ~/livereview/config/apache.conf.example /etc/apache2/sites-available/livereview.conf
sudo apache2ctl configtest
```

### Page loads, but nothing works (login fails, no data appears)

You're likely still in demo mode. See [Switch LiveReview to production mode](#4-switch-livereview-to-production-mode) — demo mode hardcodes the API URL to `localhost`, which breaks once the app is accessed through a reverse proxy from any other machine.

```
lrops.sh set-mode production
```

Hard-refresh the browser afterward (the old page may have cached the broken API URL in memory).

### `setup-ssl` fails asking for an email / hangs on a prompt

You ran it without the email argument, so `certbot` fell back to an interactive prompt that doesn't handle non-interactive input cleanly. Re-run with an email:

```
sudo lrops.sh setup-ssl yourdomain.com you@example.com
```

### `setup-ssl` fails to obtain a certificate

Almost always DNS or port `443`, not Apache. Check:

```
dig yourdomain.com                 # does it resolve to this server's IP?
curl -s ifconfig.me                # what IS this server's IP?
sudo ufw status                    # is 443 open?
```

All three must line up before `certbot` can succeed.
