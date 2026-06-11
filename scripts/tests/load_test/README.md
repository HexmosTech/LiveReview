update mockll.toml in server

rsync -avz ./internal/mockllm/mockllm.toml nats03-do:/home/ubuntu/staging_lr/internal/mockllm/mockllm.toml && ssh nats03-do "bash -ic 'cd /home/ubuntu/staging_lr && pm2 reload ecosystem.staging.config.js --update-env'"