import os
import re
import sys

def get_api_key():
    home = os.path.expanduser("~")
    toml_path = os.path.join(home, ".lrc.toml")
    if not os.path.exists(toml_path):
        print(f"[-] Config file not found at {toml_path}", file=sys.stderr)
        return None
    try:
        with open(toml_path, "r") as f:
            content = f.read()
        match = re.search(r'api_key\s*=\s*"(.*?)"', content)
        if match:
            return match.group(1)
    except Exception as e:
        print(f"[-] Error reading {toml_path}: {e}", file=sys.stderr)
    return None

def get_api_url():
    home = os.path.expanduser("~")
    toml_path = os.path.join(home, ".lrc.toml")
    if not os.path.exists(toml_path):
        return "http://localhost:8888"
    try:
        with open(toml_path, "r") as f:
            content = f.read()
        match = re.search(r'api_url\s*=\s*"(.*?)"', content)
        if match:
            return match.group(1)
    except Exception as e:
        pass
    return "http://localhost:8888"
