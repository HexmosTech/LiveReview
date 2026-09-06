The LiveReview CLI is called `lrc`. It is usually installed in `/usr/local/bin/lrc`.

`lrc` can also be invoked as a git subcommand like `git lrc`. Usually this is the recommended way to trigger `lrc` (via git).

`lrc` provides powerful global hook management for all your git repositories.

## Insallation

<img width="739" height="416" alt="image" src="https://github.com/user-attachments/assets/c22e7d60-85d1-4615-b89c-0928d62e3fdd" />

1. Goto Settings
1. Goto API Keys
1. Create New Key
1. Enter a friendly label
1. Create Key

You'll get instructions for installing the tool like so:

<img width="550" height="225" alt="image" src="https://github.com/user-attachments/assets/8dff29e9-8f9b-436e-bb5e-df42a85be3fc" />

You can copy/paste the command and get it installed. Notice that it'll configure global hooks automatically for all repos by default (don't worry - you can disable hooks on individual projects if you wish):

<img width="610" height="196" alt="image" src="https://github.com/user-attachments/assets/1e9dcfc8-549b-4c48-b955-9927325c7919" />

Now - goto a git repo, and make a change:

<img width="550" height="290" alt="image" src="https://github.com/user-attachments/assets/9d94087f-15e9-4691-b72d-dd94c94cebef" />

Now try:

```
git add .
git commit
```

This will automatically trigger the pre-commit hook:

<img width="562" height="243" alt="image" src="https://github.com/user-attachments/assets/497e7d63-365e-4d42-ae95-e965e5cfbefd" />

You can see the review in `http://localhost:8000` and it has clearly found the accidental addition we did above:

<img width="890" height="475" alt="image" src="https://github.com/user-attachments/assets/a255f70c-275c-468c-a3bb-fc856b761ca9" />

We can Commit, or Commit & Push or Skip the review right from the webui. Since this is an accidental change, we can skip the change.

<img width="600" height="100" alt="image" src="https://github.com/user-attachments/assets/98f78a56-3c7e-4f60-9168-097afd30ffcb" />

If you  want to disable lrc hooks for a given repo - just do `git lrc hooks disable`. You can find all the hook options below:

<img width="620" height="301" alt="image" src="https://github.com/user-attachments/assets/16f65165-457f-4328-b5f7-91b122f697d6" />

 