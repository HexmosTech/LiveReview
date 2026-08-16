# Upstream keepalive pools: without these, nginx opens a brand-new TCP connection to the
# single-instance Go backend for every proxied request (its default proxy_pass behavior is
# HTTP/1.0-style, connection-per-request). Invisible under light/sequential load, but under a
# burst of many simultaneous requests - e.g. right after login, when the browser fires off many
# JS chunk downloads plus API calls at once - connection setup churns across the whole burst and
# every request in it stalls, static files included (confirmed via HAR: even plain .js/.css
# requests, which touch no DB/auth code, showed the same multi-second stall as API calls, only
# during bursts). `keepalive N` here lets nginx reuse a pool of persistent connections to each
# backend instead of opening a fresh one per request. See docs/perf-improvement.md.
upstream livereview_api {
    server 127.0.0.1:8888;
    keepalive 32;
}

upstream livereview_ui {
    server 127.0.0.1:8081;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name livereview.hexmos.com;

    ssl_certificate     /etc/letsencrypt/live/livereview.hexmos.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/livereview.hexmos.com/privkey.pem;
    proxy_max_temp_file_size 0;

    # ================= API =================
    # CORS is handled entirely by the Go backend
    location /api/ {
        proxy_pass http://livereview_api;
        # Required for upstream keepalive to actually take effect - the keepalive connection
        # pool only works over HTTP/1.1 with the Connection header cleared (nginx defaults to
        # HTTP/1.0 + Connection: close otherwise, which defeats the upstream{} pool above).
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # =============== FRONTEND ===============
    location / {
        proxy_pass http://livereview_ui;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # The livereview-ui process now pre-gzips its static assets at startup and sends
        # Content-Encoding: gzip itself (cmd/ui.go, buildCompressedAssets) - nginx re-compressing
        # an already-gzipped response on every single request was the confirmed root cause of
        # multi-second stalls across entire request bursts (static files AND unrelated API calls
        # queuing behind the same CPU-bound compression work). nginx already skips gzip on a
        # response that has Content-Encoding set, but this makes the intent explicit rather than
        # relying on that. See docs/perf-improvement.md "Finding A".
        gzip off;
    }
}

server {
    listen 80;
    server_name livereview.hexmos.com;

    location /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
