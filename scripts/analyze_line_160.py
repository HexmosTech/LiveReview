#!/usr/bin/env python3
"""
Analyze which lines in a diff are added, deleted, or context lines
This helps in diagnosing GitLab line comment issues
"""

import sys
import re
import os
from colorama import init, Fore, Style

# Initialize colorama for cross-platform colored output
init()

def analyze_line_160():
    """Specific analysis for line 160 in gk_input_handler.go"""
    log_file = "hunk_format_logs/gk_input_handler.go_hunks.log"
    
    if not os.path.exists(log_file):
        print(f"Error: Could not find {log_file}")
        return
    
    with open(log_file, "r") as f:
        content = f.read()
    
    # Find the relevant hunk containing line 160
    line_160_pattern = r'\b160\|.*defer client\.Close\(\)'
    match = re.search(line_160_pattern, content)
    
    if not match:
        print("Could not find line 160 in the log file")
        return
    
    # Get context around the line
    start = max(0, match.start() - 200)
    end = min(len(content), match.end() + 200)
    context = content[start:end]
    
    print("===== ANALYSIS OF LINE 160 =====")
    print(Fore.YELLOW + "Context around line 160:" + Style.RESET_ALL)
    
    # Highlight the line
    lines = context.split('\n')
    for line in lines:
        if "160|" in line and "defer client.Close()" in line:
            print(Fore.RED + Style.BRIGHT + line + Style.RESET_ALL)
            
            # Parse the line details
            parts = line.strip().split('|')
            if len(parts) >= 3:
                old_line = parts[0].strip()
                new_line = parts[1].strip()
                content_part = '|'.join(parts[2:])
                
                print("\nLine 160 Analysis:")
                print(f"Old Line Number: {old_line}")
                print(f"New Line Number: {new_line if new_line else 'NONE (empty)'}")
                print(f"Content: {content_part}")
                
                # Determine line type
                if new_line.strip() == "" and "-" in content_part:
                    print(f"\n{Fore.RED}{Style.BRIGHT}CONCLUSION: Line 160 is a DELETED line{Style.RESET_ALL}")
                    print("For GitLab API, use 'old_line' parameter and set IsDeletedLine=true")
                    print("Line 160 should show up in the OLD version of the file only")
                elif old_line.strip() == "" and "+" in content_part:
                    print(f"\n{Fore.GREEN}{Style.BRIGHT}CONCLUSION: Line 160 is an ADDED line{Style.RESET_ALL}")
                    print("For GitLab API, use 'new_line' parameter and set IsDeletedLine=false")
                    print("Line 160 should show up in the NEW version of the file only")
                else:
                    print(f"\n{Fore.WHITE}{Style.BRIGHT}CONCLUSION: Line 160 is a CONTEXT line{Style.RESET_ALL}")
                    print("For GitLab API, you can use either parameter, but 'new_line' is preferred")
                    print("Line 160 should show up in both OLD and NEW versions of the file")
        else:
            print(line)

def main():
    analyze_line_160()

if __name__ == "__main__":
    main()
