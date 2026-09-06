## Part 1: Get a BitBucket account starting with `LiveReview` (ex: `LiveReview2125`) 

Register a new bitbucket account and add that to all the workspaces/orgs necessary.

Make sure you can access and interact with the relevant repos.

## Part 2: Get Token

Goto Atlassian Account Settings


<img width="900" height="415" alt="image" src="https://github.com/user-attachments/assets/603ae630-0e86-47b2-ad1e-c10650edd595" />


Goto Security Tab, pick "Create and Manage API Tokens"

<img width="900" height="415" alt="image" src="https://github.com/user-attachments/assets/a79f561f-823d-4954-a829-00d9793e8a18" />


Select "Create API Tokens With Scope"

<img width="900" height="315" alt="image" src="https://github.com/user-attachments/assets/ad36e962-1698-4629-a43f-c63299114865" />

Give a name, and expiry date

<img width="900" height="715" alt="image" src="https://github.com/user-attachments/assets/238fb6f5-9ac5-41aa-b53d-ffdbba9d53f3" />


Select BitBucket

<img width="900" height="715" alt="image" src="https://github.com/user-attachments/assets/2e1def85-4f1d-4cc8-a219-456e46b20a07" />

Search and select a scope

<img width="900" height="415" alt="image" src="https://github.com/user-attachments/assets/dea31b0b-7c66-4843-a332-f864344430ef" />

Add all 7 of the following scopes:


Here are those scopes
```
read:workspace:bitbucket
read:user:bitbucket
read:me
read:account
read:pullrequest:bitbucket
read:repository:bitbucket
read:webhook:bitbucket
write:webhook:bitbucket
```


<img width="900" height="415" alt="image" src="https://github.com/user-attachments/assets/be003977-8d48-42db-a3b9-7554855b7e88" />

Copy API token

<img width="900" height="615" alt="image" src="https://github.com/user-attachments/assets/753df360-a2fa-44ea-9b07-5908a0e0ada4" />


## Part 3: Add to LiveReview

In LiveReview Goto Git Providers -> Bitbucket

<img width="900" height="415" alt="image" src="https://github.com/user-attachments/assets/62f9d28f-d23d-423d-98f8-f516948b337c" />

Enter the API Token and and give a friendly name

<img width="900" height="815" alt="image" src="https://github.com/user-attachments/assets/d800955b-bef4-4493-a992-b790a01d164b" />


Confirm and save

<img width="900" height="815" alt="image" src="https://github.com/user-attachments/assets/24b44002-e419-4285-8a60-239dcfe27c12" />


