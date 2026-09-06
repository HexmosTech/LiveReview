To update LiveReview, first update the lrops.sh script:

```
sudo lrops.sh self-update
```

Then update the containers:

```
# Update to latest version:
sudo lrops.sh update
```

or you can update to a specific version

```
# update to a specific version:
sudo lrops.sh update v1.0.0
```

This will create a pre-update backup, pull the latest image, restart containers, and give you the latest version.

If anything goes wrong, you can restore with:

```
sudo lrops.sh restore latest
```