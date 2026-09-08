### Step 1: Run the installation script

```
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | sudo bash -s -- setup-production
```

### Step 2: Enter database and JWT

<img width="960" height="339" alt="image" src="https://github.com/user-attachments/assets/1a08238d-2862-42d0-9a4b-961683750854" />

### Step 3: Wait for the installation to complete

<img width="897" height="446" alt="image" src="https://github.com/user-attachments/assets/9d38bd71-e259-4668-a574-62b0710b7c9d" />

### Step 4: Complete the prerequisites
These information can also be viewed via the `lrops.sh help caddy` command

#### 1. Verify your domain points to this server
#####  a) Get your server's public IP address:

```
curl -s ifconfig.me
# OR: curl -s ipinfo.io/ip
```

##### b) Check DNS resolution locally:
```
dig yourdomain.com
nslookup yourdomain.com
```

   
##### c) Verify DNS propagation globally (CRITICAL):
- Visit: https://www.whatsmydns.net/
- Enter your domain name
- Select "A" record type
- Confirm ALL locations show your server's IP
      
##### d) Alternative DNS propagation check:
- Visit: https://dnschecker.org/
- Enter your domain and verify worldwide propagation
   
##### e) Command-line verification from different locations:

```
# Use different DNS servers to check consistency
dig @8.8.8.8 yourdomain.com        # Google DNS
dig @1.1.1.1 yourdomain.com        # Cloudflare DNS  
dig @208.67.222.222 yourdomain.com # OpenDNS
```

   
##### Common mistakes to avoid
- Don't proceed if DNS shows different IPs in different locations
- Wait for full global propagation (can take up to 48 hours)
- Ensure you're checking the RIGHT domain (not www. vs non-www)
- Verify both A record AND any CNAME records point correctly

#### 2. Verify network connectivity

#####  a) Check ports 80 and 443 are accessible from internet:
```
# From another machine/location, test:
telnet yourdomain.com 80
telnet yourdomain.com 443
```
##### b) Check firewall rules:
```
sudo ufw status
# Ensure ports 80 and 443 are allowed
```  
#####  c) Check cloud security groups (AWS/GCP/Azure/DigitalOcean):
- Verify inbound rules allow TCP ports 80 and 443 from 0.0.0.0/0
   
#####  d) Test with online port checker:
- Visit: https://www.yougetsignal.com/tools/open-ports/
- Enter your domain and test ports 80, 443

#### 3. Verify no port conflicts
##### a) Check nothing else is using ports 80/443:

```
sudo ss -tlnp | grep ':80\|:443'
sudo netstat -tlnp | grep ':80\|:443'
```

   
##### b) If Apache/nginx already running, you'll need to:
- Stop them temporarily, OR
- Configure them as the reverse proxy (recommended)

#### 4. Final verification checklist
- Domain resolves to correct IP globally (whatsmydns.net shows green)
- Ports 80 and 443 are open from internet (telnet/port checker works)  
- No services currently using ports 80/443 (ss/netstat shows clear)
- LiveReview is running and accessible on ports 8888/8081 locally

```
curl http://localhost:8888/health    # Should return OK
```

```
curl http://localhost:8081/          # Should return HTML
```

#### Note
- Without proper DNS pointing to your server, SSL certificates cannot be obtained.
- Let's Encrypt and other CAs verify domain ownership by checking that your domain resolves to the requesting server.

#### Troubleshooting DNS Issues
- If DNS propagation is incomplete, wait - don't proceed
- If different regions show different IPs, contact your DNS provider
- If using Cloudflare, ensure proxy is disabled (gray cloud) for SSL setup
- Check TTL settings - lower TTL (300-900 seconds) speeds up changes




---



### Step 5: Install Caddy
Run the following commands to get Caddy installed

```
sudo apt update
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy
```

sudo systemctl status caddy
### Step 6: Copy and configure the caddy template

```
sudo cp ~/livereview/config/caddy.conf.example /etc/caddy/Caddyfile
sudo sed -i 's/your-domain.com/your-actual-domain.org/g' /etc/caddy/Caddyfile
```

#### Example
If your domain is `livereview.google.com`, then you have to replace `your-actual-domain` with your domain

So the command becomes

```
sudo sed -i 's/your-domain.com/livereview.google.com/g' /etc/caddy/Caddyfile
```


### Step 7: Enable Caddy and apply the configuration

Run these commands

```
sudo systemctl enable caddy
sudo systemctl reload caddy
```



### Step 8: Test whether everything works properly

#### 1. Check Caddy Status

```
sudo systemctl status caddy
```


<img width="1920" height="278" alt="image" src="https://github.com/user-attachments/assets/bf4a2f40-460f-48df-83f3-13fadeb36840" />

#### 2. View the logs

```
sudo journalctl -u caddy -f
```


<img width="1917" height="103" alt="image" src="https://github.com/user-attachments/assets/7fd9b22d-eb6f-4754-9115-693a37410cf4" />


#### 3. Test the proxy

```
curl https://yourdomain.com/
```
