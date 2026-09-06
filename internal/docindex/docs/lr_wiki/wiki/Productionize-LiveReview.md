
## Get Demo Mode Running

You can follow the article for [Demo Mode](Download,-Install-and-Run-LiveReview) and make sure the software is running first in your target server.

When you run `lrops.sh status` you should something like as follows:
 
<img width="903" height="394" alt="image" src="https://github.com/user-attachments/assets/ccdd35e2-9d49-4df0-b106-4e7b5f2c27a6" />

## Update Configuration

The next task is to turn on the production mode. 

Run `lrops.sh set-mode prod` and enter "y" to continue:

<img width="900" height="650" alt="image" src="https://github.com/user-attachments/assets/44665ee3-2596-4e0a-b5a0-0b00aad8a8f9" />

You should see success message:

<img width="900" height="180" alt="image" src="https://github.com/user-attachments/assets/2bdfded6-09b1-40dc-9f23-776cc89f7d69" />


Run `lrops.sh show-mode` to confirm:

<img width="900" height="398" alt="image" src="https://github.com/user-attachments/assets/a89bf1cb-eadf-47e3-a5a4-c75cddcad280" />

## Setup Reverse Proxy

The goal now is to:

1. Point `yourdomain.com` or `livereview.yourdomain.com` to your production server
2. Get a reverse proxy to handle requests
3. Pass the requests correctly to our docker containers
4. Make sure SSL (https) works correctly

This can be achieved in many ways. For reverse proxy you can use nginx, caddy, apache or any other tool of your
choice.

The first task is to setup an `A` record in your DNS manager such as Namecheap, GoDaddy or AWS Route 53, etc 
so that `yourdomain.com` or `livereview.yourdomain.com` routes traffic to your server's public IP `A.B.C.D`.

For example, in `hexmos.com` case - you can see the namecheap configuration as follows:

<img width="850" height="410" alt="image" src="https://github.com/user-attachments/assets/2ac5bca6-7e3e-4e79-995b-ad299884ca6f" />

You can use [DNS Propagation Checker](https://www.google.com/search?q=dns+propagation&prmoud=ivns&sxsrf=AE3TifN_MBHxhptw3SIfZL21ExkeZRECVw:1757671771249&dpr=1.5) or `dig` command to validate this.

It looks like this in our case - you can still a few locations still waiting to get propagated:

<img width="900" height="440" alt="image" src="https://github.com/user-attachments/assets/9ccac40b-2715-45dc-9f69-fa1223607d64" />


Run `lrops.sh help nginx` or `lrops.sh help caddy` or `lrops.sh help apache` to get reverse proxy-specific
instructions on how to route this traffic to the containers.

High level instructions for nginx are as follows:

```
1. Install Nginx:
   sudo apt update && sudo apt install nginx

2. Copy the LiveReview Nginx template:
   sudo cp ~/livereview/config/nginx.conf.example /etc/nginx/sites-available/livereview.conf

3. Edit the domain name:
   sudo sed -i 's/your-domain.com/your-actual-domain.org/g' /etc/nginx/sites-available/livereview.conf
   e.g. for livereview.google.com:
   sudo sed -i 's/your-domain.com/livereview.google.com/g' /etc/nginx/sites-available/livereview.conf

4. Enable the site:
   sudo ln -s /etc/nginx/sites-available/livereview.conf /etc/nginx/sites-enabled/
   sudo nginx -t
   sudo systemctl reload nginx
```

Use the `.conf` suffix for the filename. Some nginx builds include site files with
`include sites-enabled/*.conf`, and a file without the extension is then silently ignored -
`nginx -t` still passes and nothing appears to be wrong.

### Check your work with `lrops.sh doctor`

After each step you can run:

```
lrops.sh doctor yourdomain.com
```

It tests each layer separately - the app, this machine's nginx on port 80 and on port 443, and
what the public internet actually receives - and then tells you which layer is the problem.
Most "the site is down" situations are just those layers disagreeing:

```
  app  :8888/health   200
  app  :8081/         200
  origin :80          200
  origin :443         -
  public https://     521
```

At this point - you can actually visit the URL to get the http (insecure) version of the app:

<img width="950" height="480" alt="image" src="https://github.com/user-attachments/assets/f0b7d51f-b99f-48e6-b9ca-790e15211c7c" />


Finally - you will need SSL certificates to secure the configuration. You can do `lrops.sh help ssl` to get
some guidance. Usually you can use [let's encrypt](https://letsencrypt.org/) to get a certificate for free
using their `certbot` command. You can also probably get it via Cloudflare or your organization's administrators
can help you get a certificate.

Here are general guidance on getting SSL certificate and configuring:

```
OPTION 1: Automatic SSL with Caddy (Recommended for new setups)
- Handles certificates automatically
- Zero manual certificate management
- See: lrops.sh help caddy

OPTION 2: Manual SSL with existing reverse proxy
- Use your existing nginx/apache setup
- Obtain certificates with certbot or your preferred method
- Configure your reverse proxy to use certificates

OPTION 3: Cloud/managed SSL
- Use CloudFlare, AWS ALB, or similar services
- Terminate SSL at the load balancer/CDN level
- Point to your LiveReview ports (8888/8081)

REQUIREMENTS FOR ALL APPROACHES:
- Domain pointing to your server (DNS setup)
- Ports 80 and 443 accessible
- LiveReview running on ports 8888 (API) and 8081 (UI)

REVERSE PROXY ROUTING:
Route /api/* → http://127.0.0.1:8888
Route /* → http://127.0.0.1:8081

GENERAL SSL GUIDANCE:
- Let's Encrypt is free and widely supported
- Use certbot for most manual SSL setups
- Configure automatic certificate renewal
- Test your SSL setup: https://www.ssllabs.com/ssltest/
```

In my particular case, I had to the following:

Get the certificate only:

```
sudo certbot certonly -d livereview.hexmos.com
```

Once the certificate was ready, I opened up the configuration file:

```
vim /etc/nginx/sites-enabled/livereview.conf

# deleted the http-only block, uncommented the https block (there's already a template)

nginx -t # returned OK, no syntax errors
sudo systemctl restart nginx
```

This is a pretty standard thing to do for setting up any website or webapp. If you are facing any issues in
your particular circumstance in getting this to work - just [Create an Issue](https://github.com/HexmosTech/LiveReview/issues) and
our team will help you out.

The result of this step is that you can open `https://yourdomain.com` or `https://livereview.yourdomain.com` to
get the login screen for LiveReview. And when you look at the URL bar - you can see that there is a valid SSL
certificate protecting transactions with the service.

I did a hard reload for `https://livereview.hexmos.com` in my browser, and I got the app as expected:

<img width="950" height="475" alt="image" src="https://github.com/user-attachments/assets/83e9f785-e22d-4c1b-abbc-6dd4dc7dece8" />


### If your domain is behind Cloudflare (or another CDN)

This is worth reading before you conclude something is broken. If your domain is proxied
through Cloudflare, finishing the four nginx steps above can still leave you looking at an
error page, because those steps produce an **HTTP-only** origin while Cloudflare, depending on
its SSL/TLS mode, connects to your server over **HTTPS on port 443**.

Cloudflare's own error code tells you exactly what is wrong:

| What you see | What it means | Fix |
| --- | --- | --- |
| **521** | Cloudflare is connecting over HTTPS, but your server has nothing listening on port 443 | Complete the SSL step below, or set Cloudflare's SSL mode to *Flexible* |
| **526** | Your origin certificate is not trusted - Cloudflare is on *Full (strict)* and your certificate is self-signed | Use a real certificate (Let's Encrypt) |
| **525** | The TLS handshake with your origin failed | Check `ssl_certificate` / `ssl_certificate_key` paths |
| **522** | Cloudflare could not reach your server in time | Usually a firewall blocking Cloudflare's IPs - check `sudo ufw status` |

The most reliable setup, and the one we recommend, is to **always terminate TLS on your own
server** with a real certificate. That single configuration is correct whether or not a CDN is
in front of you, and whichever SSL mode it uses:

| Your setup | Origin with port 80 + port 443 and a real certificate |
| --- | --- |
| No CDN, DNS points straight at the server | Works |
| Cloudflare, *Flexible* mode | Works (Cloudflare uses port 80) |
| Cloudflare, *Full* mode | Works (Cloudflare uses port 443) |
| Cloudflare, *Full (strict)* mode | Works (the certificate is trusted) |

An HTTP-only origin fails two of those four, and a self-signed certificate fails one - which is
why it is worth doing properly the first time rather than discovering it later.

One thing to keep in mind while issuing a certificate: Let's Encrypt's HTTP-01 challenge has to
reach `/.well-known/acme-challenge/` on your server. The bundled nginx template keeps that path
served over plain HTTP for exactly this reason, so leave it in place. If your CDN blocks it, use
a DNS-01 challenge instead.

### A note on direct port access

The containers publish ports 8081 and 8888 on all interfaces, so `http://YOUR_SERVER_IP:8081`
reaches the app directly - bypassing your reverse proxy, your TLS, and your CDN. Before going
live, either firewall those ports or bind them to localhost in `~/livereview/docker-compose.yml`:

```
ports:
    - "127.0.0.1:8081:8081"
    - "127.0.0.1:8888:8888"
```

Then `cd ~/livereview && docker compose up -d --force-recreate`. `lrops.sh doctor` warns you if
it detects this.

## Set Production URL

In production, if you try to add a Git connector, it'll ask you to configure a domain URL first:

<img width="950" height="440" alt="image" src="https://github.com/user-attachments/assets/3fd7caa5-f017-4aaa-8a00-091631ea32a2" />

The reason for this is - because we want to configure how platforms such as Gitlab/Github/Bitbucket can send webhooks/notifications to our server. 

You can Goto `Settings -> Instance`. There usually the path will be pre-filled via the address bar. If not you can type it out and hit "Save":

<img width="950" height="420" alt="image" src="https://github.com/user-attachments/assets/d0b14802-93ff-4f03-8fe3-d0909972b093" />

Now if you come back to Git connectors, you will have the buttons visible and the warnings will be gone:

<img width="950" height="400" alt="image" src="https://github.com/user-attachments/assets/1c452876-d14c-4a6d-acb4-ade8f87f8c1d" />


## Update Webhooks

You can follow [Add Git Providers](Adding-Git-Providers-to-LiveReview) page if you haven't already to add at least 1 git connector to your LiveReview.

You should have at least 1 connector like this in the "Your Connectors" section; click the "Cog" icon to see settings for that connector

<img width="950" height="435" alt="image" src="https://github.com/user-attachments/assets/ed34ddae-d737-47eb-8b4b-d407993a0545" />

You'll see the "Repository Access" section:

<img width="800" height="417" alt="image" src="https://github.com/user-attachments/assets/a2c23a45-3800-4925-87a8-ca572184762b" />


Here, we have three categories possible:

1. **Not connected:** It means - the only way to trigger a review is by using LiveReview's web UI with "New Review"
1. **Manual Trigger:** You can assign a reviewer in the Gitlab/Github/BitBucket MR pages and LiveReview will pick it up and post review comments.
1. **Automatic:** LiveReview will automatically trigger a review when it is opened or a change is posted to it. **As of now, this feature is not implemented yet.**

To enable manual trigger, if it is not already enabled, just click on "Enable Manual Trigger for All Projects"

<img width="900" height="175" alt="image" src="https://github.com/user-attachments/assets/6052b801-7534-4e84-8d83-f505ee9bd95c" />

In the panel below - you can see project level progress:

<img width="800" height="350" alt="image" src="https://github.com/user-attachments/assets/7bc0798d-db8e-41df-a570-ea02521f8c1b" />


## Trigger Review via Reviewer Assignment


You can trigger a review now by assigning a reviewer in the code host's MR interface:

<img width="950" height="411" alt="image" src="https://github.com/user-attachments/assets/fcfb4938-1414-4ce1-9d18-59e3d0ed8940" />

