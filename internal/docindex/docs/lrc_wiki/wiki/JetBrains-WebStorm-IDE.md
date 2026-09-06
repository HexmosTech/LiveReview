# How to Trigger Local Code Review Using git-lrc in WebStorm IDE

Here are the instructions on how to trigger a local code review using git-lrc in your WebStorm IDE environment.

## Prerequisites

1. The instructions below only work if you have installed and set up git-lrc on your local machine.

## Method 1: Using WebStorm's Built-in Terminal

Once your code changes are ready for commit, follow these commands from the WebStorm built-in terminal:

1. **Stage your changes:**
```bash
   git add .
```

2. **Commit with a message:**
```bash
   git commit -m "commit message"
```

3. **Review automatically starts:**  
   git-lrc will automatically spin up a local server, and you can review the code comments from there.

<img width="873"  alt="image" src="https://github.com/user-attachments/assets/dec14642-9c1e-4160-8ab7-ccdb9bf619c8" />

4. **Complete the review:**  
   Once reviewed, you can either continue with the commit or abort it—either from the review UI running on localhost or from the terminal itself.

<img width="873" alt="image" src="https://github.com/user-attachments/assets/c403f2b2-591a-4fe6-93ee-1dff710875e4" />


## Method 2: Using WebStorm's Git Tool

WebStorm has Git tools integrated into the platform. If you're committing your changes using this option, here's the flow:

<img width="497" height="735" alt="image" src="https://github.com/user-attachments/assets/1ab4c33e-c414-45b8-b5f7-0fd0e9332534" />

1. **Start the review from terminal:**
```bash
   lrc review --staged
```
   (You can also use `--skip` or `--vouch` flags)

2. **Review in browser:**  
   git-lrc starts, and you can see the review/comments in the localhost web UI.

3. **Commit from WebStorm:**  
   Once reviewed, add a commit message in the Git tools commit box and click **Commit** or **Commit & Push**.
<img width="503" height="301" alt="image" src="https://github.com/user-attachments/assets/1bbf8af8-316e-4f28-9212-22c78b62b5f4" />
