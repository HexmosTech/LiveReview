# Part 1: Prepare a Custom User for LiveReview  

Get an account in gitlab.com 

---

## 1.2 Grant the User Access to Relevant Groups/Projects  

Manually go to the appropriate projects/groups and add the new user as a member:  

<img width="800" height="350" alt="image" src="https://github.com/user-attachments/assets/76236f69-d663-4def-b2a1-57a45505b1d5" />  

Assign at least **Developer** or **Maintainer** permissions:  

<img width="540" height="502" alt="image" src="https://github.com/user-attachments/assets/1dd8b073-9abf-4cc7-9a1d-4b6c040d11d8" />  

You can then confirm their access in the Admin area:  

<img width="700" height="150" alt="image" src="https://github.com/user-attachments/assets/fbf035dc-3c5f-474b-9068-030f9392bf27" />  

---

## 1.3 Generate a Personal Access Token (PAT)  

The next step is to generate a PAT for the new user so LiveReview can act on their behalf.  

If you’re an admin, one option is to *impersonate* the new user:  

<img width="700" height="170" alt="image" src="https://github.com/user-attachments/assets/9b96d6f3-0c21-4410-99d4-6970581d8f84" />  

If impersonation is not possible, simply log in as the new user with their email and password.  

Navigate to:  
`User Profile Icon -> Preferences -> Access Tokens -> Add New Token`  

<img width="900" height="450" alt="image" src="https://github.com/user-attachments/assets/96e64cea-6db8-46c5-9bc2-9599e6ca1249" />  

Assign the required permissions and create a new PAT:  

<img width="600" height="400" alt="image" src="https://github.com/user-attachments/assets/996f24f4-434b-4a62-b7cc-4d2aef676158" />  

Copy the PAT to a secure location for later use.  

---

# Part 2: Connect LiveReview with the New User’s PAT  

Log in to LiveReview and go to:  
`Git Providers -> Gitlab.com`  

<img width="950" height="400" alt="image" src="https://github.com/user-attachments/assets/1307f6d7-9633-482f-a709-15aef06a36e5" />
  

Fill in the details:  
- Provide a friendly name.  
- Enter the PAT.  
- Submit.  

<img width="800" height="550" alt="image" src="https://github.com/user-attachments/assets/d448c19a-1c14-4e77-a9b6-5f26bf27ae50" />
  

LiveReview should fetch the user’s profile (avatar) and some basic details, like this:  

<img width="871" height="648" alt="image" src="https://github.com/user-attachments/assets/4875aea2-566c-4901-a8b8-6f0b3f343beb" />  

Finally, click **Confirm and Save** to add the connector:  

<img width="850" height="300" alt="image" src="https://github.com/user-attachments/assets/79c282b6-a4ad-4d94-a8fa-161377c33479" />  
