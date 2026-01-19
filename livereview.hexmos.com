server {
    listen 443 ssl http2;
    server_name livereview.hexmos.com;

    ssl_certificate     /etc/letsencrypt/live/livereview.hexmos.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/livereview.hexmos.com/privkey.pem;
    proxy_max_temp_file_size 0;

    # ================= API =================
    # CORS is handled entirely by the Go backend
    location /api/ {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # =============== FRONTEND ===============
    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}