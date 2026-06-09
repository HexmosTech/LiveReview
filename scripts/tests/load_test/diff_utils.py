import os
import io
import sys
import zipfile
import base64

def parse_diff_loc(content):
    additions = 0
    deletions = 0
    for line in content.splitlines():
        if line.startswith("+") and not line.startswith("+++"):
            additions += 1
        elif line.startswith("-") and not line.startswith("---"):
            deletions += 1
    return additions, deletions

def make_zip_from_test_repo():
    repo_path = "/home/lince/hexmos/test-repo-1000loc"
    if not os.path.exists(repo_path):
        print(f"[-] Test repository not found at {repo_path}", file=sys.stderr)
        return None
    try:
        import subprocess
        # Get the diff of the last commit
        diff_content = subprocess.check_output(
            ["git", "diff", "HEAD~1"], cwd=repo_path
        ).decode("utf-8")
        
        adds, dels = parse_diff_loc(diff_content)
        file_loc = adds + dels
        
        print(f"\n[+] Analyzing external test repository diff in {repo_path}:")
        print(f"  • test_repo_1000loc.diff -> Additions: {adds:<3} | Deletions: {dels:<3} | LOC: {file_loc}")
        print(f"[+] Total Package Metrics -> Files: 1 | Total Adds: {adds} | Total Dels: {dels} | Total LOC: {file_loc}\n")
        
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
            z.writestr("test_repo_1000loc.diff", diff_content)
            
        return base64.b64encode(buf.getvalue()).decode("utf-8")
    except Exception as e:
        print(f"[-] Error generating diff from {repo_path}: {e}", file=sys.stderr)
        return None

def make_zip_from_samples():
    script_dir = os.path.dirname(os.path.realpath(__file__))
    samples_dir = os.path.join(script_dir, "sample_diffs")
    
    if not os.path.exists(samples_dir) or not os.path.isdir(samples_dir):
        print(f"[-] Sample diffs directory not found at: {samples_dir}", file=sys.stderr)
        return None

    diff_files = [f for f in os.listdir(samples_dir) if f.endswith(".diff") or f.endswith(".patch")]
    if not diff_files:
        print(f"[-] No .diff or .patch files found in: {samples_dir}", file=sys.stderr)
        return None

    print(f"\n[+] Analyzing sample diff files in {samples_dir}:")
    total_adds = 0
    total_dels = 0
    
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for filename in sorted(diff_files):
            file_path = os.path.join(samples_dir, filename)
            try:
                with open(file_path, "r", encoding="utf-8") as f:
                    content = f.read()
                
                adds, dels = parse_diff_loc(content)
                file_loc = adds + dels
                total_adds += adds
                total_dels += dels
                
                print(f"  • {filename:<15} -> Additions: {adds:<3} | Deletions: {dels:<3} | LOC: {file_loc}")
                z.writestr(filename, content)
            except Exception as e:
                print(f"[-] Error reading/adding {filename}: {e}", file=sys.stderr)
                return None
                
    total_loc = total_adds + total_dels
    print(f"[+] Total Package Metrics -> Files: {len(diff_files)} | Total Adds: {total_adds} | Total Dels: {total_dels} | Total LOC: {total_loc}\n")
    return base64.b64encode(buf.getvalue()).decode("utf-8")
