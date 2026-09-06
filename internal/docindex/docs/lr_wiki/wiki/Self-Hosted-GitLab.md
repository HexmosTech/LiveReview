# Part 1: Prepare a Custom User for LiveReview  

## 1.1 Create a User Starting with `LiveReview`  
(Example: `LiveReviewBot` or `LiveReview1914`)  

If you're the admin, one way to create a user is via the Admin area (see steps below). Other methods are also described in the [GitLab Docs](https://docs.gitlab.com/user/profile/account/create_accounts/).  

### Create a User in the Admin Area  
[GitLab Docs Reference](https://docs.gitlab.com/user/profile/account/create_accounts/#create-a-user-in-the-admin-area)  

**Prerequisites:**  
- You must be an administrator for the instance.  

**Steps:**  
1. On the left sidebar, at the bottom, select **Admin**.  
2. Select **Overview > Users**.  
3. Select **New user**.  
4. In the **Account** section, enter the required account information.  
5. (Optional) In the **Access** section, configure any project limits or user type settings.  
6. Select **Create user**.  

GitLab sends an email to the user with a sign-in link. The user must create a password when they first sign in. You can also directly [set a password](https://docs.gitlab.com/security/reset_user_password/#use-the-ui) for the user.  

<img width="750" height="440" alt="image" src="https://github.com/user-attachments/assets/f7dbf379-455f-43cc-9002-aa8fc4215d44" />  

<img width="668" height="290" alt="image" src="https://github.com/user-attachments/assets/3ec472f3-8de4-4cfc-8b54-20780bdacf0c" />  

If you created a regular user, you may notice they don’t yet have access to any groups/projects:  

<img width="750" height="450" alt="image" src="https://github.com/user-attachments/assets/c82c935f-66c2-4196-bbb5-595c2f77b917" />  

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
`Git Providers -> Self-Hosted GitLab`  

<img width="850" height="360" alt="image" src="https://github.com/user-attachments/assets/9ceb5925-cb74-4832-9bd5-8ee8979decb5" />  

Fill in the details:  
- Provide a friendly name.  
- Enter the PAT.  
- Add the instance URL.  
- Submit.  

<img width="981" height="759" alt="image" src="https://github.com/user-attachments/assets/674feb5d-c1df-40f0-81b6-817a88af4d15" />  

LiveReview should fetch the user’s profile (avatar) and some basic details, like this:  

<img width="871" height="648" alt="image" src="https://github.com/user-attachments/assets/4875aea2-566c-4901-a8b8-6f0b3f343beb" />  

Finally, click **Confirm and Save** to add the connector:  

<img width="850" height="300" alt="image" src="https://github.com/user-attachments/assets/79c282b6-a4ad-4d94-a8fa-161377c33479" />  
