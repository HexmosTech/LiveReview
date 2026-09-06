By default - when LiveReview is created - we create a mandatory "Super Admin". But LiveReview comes with a more granular access model - consisting of:

1. **Super Admin:** Controller of the instance. Can create orgs, owners, members. Can configure instance settings (demo mode vs production mode), set production URL and so on.
1. **Owner:** Has control over one or more orgs - as specified by the Super Admin. Owner can manage teams for the organization they control.
1. **Member:** Gets access to specified org dashboards and review functionality. Cannot manage team.

To access team management, be logged in as either a Super Admin or Owner first. 

The first thing to note here is the "Org Selector" dropdown in (1) below. Each team management action applies only to that org selected in the dropdown.

Once the org is selected move to `Settings -> User Management -> Add User`

<img width="950" height="425" alt="image" src="https://github.com/user-attachments/assets/333ea373-bc56-4c99-9d71-b5837603b7ab" />

You can create a new user in this form, and add. I will add an admin to the given org.

<img width="900" height="490" alt="image" src="https://github.com/user-attachments/assets/020e4111-11a0-418f-a043-12951ec5497e" />

Once I hit "Add User", we get the new user in the list. They can login with the newly specified email and password now from the login page.

<img width="900" height="325" alt="image" src="https://github.com/user-attachments/assets/cff66698-8ce5-4063-8c96-181a4c1627b8" />

