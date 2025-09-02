export CR_PAT=REDACTED_GITHUB_PAT_1
echo "$CR_PAT" | docker login ghcr.io -u shrsv --password-stdin
