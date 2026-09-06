LiveReview is built to run in a Linux operating system or environment.

It assumes Docker (with `docker-compose` support) is available in the system. I recommend installing Docker first from [official site](https://docs.docker.com/engine/install/ubuntu/). If you're on Ubuntu - we recommend using `apt` based installed rather than `snap` based ones. Although - we do have made it work on `snap` based ones - they're not ideal due to the various limitations they pose.

If you're ready with docker, start the app like this:

```
# Quick demo setup (localhost only, no webhooks)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | sudo bash -s -- setup-demo

# Or use the express flag (same as demo mode)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | sudo bash -s -- --express
```

It'll ask for your `sudo` password and will download all the necessary containers, configuration and so on for you to get basic access. Here I am adding `--force` to the end of command because I want to replace an existing installation:

<img width="1203" height="701" alt="image" src="https://github.com/user-attachments/assets/08235680-a457-4988-bbf0-1afb40e27731" />

Once installation is complete - you'll get details on where you can access the app:


<img width="1215" height="769" alt="image" src="https://github.com/user-attachments/assets/3d3c0464-a6f9-4e8e-972c-18e1ffc001d6" />


You can usually access the UI at `http://localhost:8081` as follows - enter the details, to get get started:

<img width="950" height="470" alt="image" src="https://github.com/user-attachments/assets/3ff5de0b-00ab-46b9-8928-4c31654f750c" />

On first login - you'll get a notice on "Demo Mode". What this means is - the app provides basic review facilities, but some special webhook related features won't be available. Don't worry about it now - you can productionize app later. For now - close that window.

<img width="950" height="435" alt="image" src="https://github.com/user-attachments/assets/979fdea1-dd14-4174-b4c7-b66cf060af1a" />

You should see the license window - you can paste the Licence [from previous step](Get-a-LiveReview-Licence) here (or click "Get Licence" link there to get a License)

<img width="810" height="538" alt="image" src="https://github.com/user-attachments/assets/4c32b9b8-65d6-47e1-8e5e-a9e098538048" />

Enter the license and hit save - you'll get something like this:

<img width="850" height="435" alt="image" src="https://github.com/user-attachments/assets/57fdcdce-0ea9-4bc7-81c6-e66d9aa8673c" />

