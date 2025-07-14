#!/usr/bin/env python3
"""
Advanced debugging script for line detection issues in GitLab line comments.
This script analyzes hunk logs and helps visualize which lines are added/deleted.
"""

import sys
import re
import os
import argparse
from colorama import init, Fore, Style

# Initialize colorama for cross-platform colored output
init()

def parse_hunk_log(log_file):
    """Parse a hunk log file and extract line information"""
    if not os.path.exists(log_file):
        print(f"Error: Hunk log file {log_file} not found")
        return None
    
    with open(log_file, 'r') as f:
        content = f.read()
    
    # Extract hunk headers and content
    hunks = []
    
    # Look for standard unified diff hunk headers
    hunk_pattern = r'@@ -(\d+),(\d+) \+(\d+),(\d+) @@([\s\S]+?)(?=@@ -\d+,\d+ \+\d+,\d+ @@|$)'
    matches = re.finditer(hunk_pattern, content)
    
    for match in matches:
        old_start = int(match.group(1))
        old_count = int(match.group(2))
        new_start = int(match.group(3))
        new_count = int(match.group(4))
        hunk_content = match.group(5).strip()
        
        hunks.append({
            'old_start': old_start,
            'old_count': old_count,
            'new_start': new_start,
            'new_count': new_count,
            'content': hunk_content,
            'lines': hunk_content.split('\n')
        })
    
    return hunks

def analyze_hunk_lines(hunks):
    """Analyze hunk lines to determine line types"""
    line_info = {}
    
    for hunk in hunks:
        old_line = hunk['old_start']
        new_line = hunk['new_start']
        
        for line in hunk['lines']:
            if not line:
                continue
                
            if line.startswith('-'):
                # Deleted line (exists in old file only)
                line_info[old_line] = {
                    'old_line': old_line,
                    'new_line': 0,
                    'content': line[1:],
                    'type': 'deleted'
                }
                old_line += 1
            elif line.startswith('+'):
                # Added line (exists in new file only)
                line_info[new_line] = {
                    'old_line': 0,
                    'new_line': new_line,
                    'content': line[1:],
                    'type': 'added'
                }
                new_line += 1
            else:
                # Context line (exists in both files)
                line_info[new_line] = {
                    'old_line': old_line,
                    'new_line': new_line,
                    'content': line,
                    'type': 'context'
                }
                old_line += 1
                new_line += 1
    
    return line_info

def display_line_info(line_info, target_lines=None):
    """Display line information with color coding"""
    print("\n=== LINE INFORMATION ===")
    print(f"{'TYPE':<10} {'OLD #':<6} {'NEW #':<6} CONTENT")
    print("-" * 80)
    
    # Sort by new line number, then old line number
    sorted_lines = sorted(line_info.values(), key=lambda x: (x['new_line'] if x['new_line'] > 0 else float('inf'), 
                                                           x['old_line'] if x['old_line'] > 0 else float('inf')))
    
    for info in sorted_lines:
        line_type = info['type']
        old_line = str(info['old_line']) if info['old_line'] > 0 else '-'
        new_line = str(info['new_line']) if info['new_line'] > 0 else '-'
        content = info['content']
        
        # Highlight if this is a target line
        is_target = False
        if target_lines:
            if info['old_line'] in target_lines or info['new_line'] in target_lines:
                is_target = True
        
        # Color based on line type
        if line_type == 'deleted':
            color = Fore.RED
        elif line_type == 'added':
            color = Fore.GREEN
        else:
            color = Fore.WHITE
        
        # Highlight target lines
        if is_target:
            print(f"{Style.BRIGHT}{color}{line_type:<10} {old_line:<6} {new_line:<6} {content[:60]}{Style.RESET_ALL}")
            print(f"  ↳ {Style.BRIGHT}TARGET LINE{Style.RESET_ALL} - GitLab requires {'old_line' if line_type == 'deleted' else 'new_line'} parameter")
        else:
            print(f"{color}{line_type:<10} {old_line:<6} {new_line:<6} {content[:60]}{Style.RESET_ALL}")

def find_line_by_content(line_info, content_snippet):
    """Find line by content snippet"""
    matching_lines = []
    
    for line_num, info in line_info.items():
        if content_snippet.lower() in info['content'].lower():
            matching_lines.append(info)
    
    return matching_lines

def main():
    parser = argparse.ArgumentParser(description='Debug GitLab line comments')
    parser.add_argument('log_file', help='Path to the hunk log file')
    parser.add_argument('--line', '-l', type=int, action='append', help='Target line number to highlight (can be used multiple times)')
    parser.add_argument('--search', '-s', help='Search for content in the diff')
    args = parser.parse_args()
    
    hunks = parse_hunk_log(args.log_file)
    if not hunks:
        sys.exit(1)
    
    print(f"Found {len(hunks)} hunks in {args.log_file}")
    
    line_info = analyze_hunk_lines(hunks)
    
    # If search term provided, search for content
    if args.search:
        print(f"\nSearching for: {args.search}")
        matching_lines = find_line_by_content(line_info, args.search)
        if matching_lines:
            print(f"Found {len(matching_lines)} matching lines:")
            for line in matching_lines:
                print(f"  {line['type']} line: old={line['old_line']}, new={line['new_line']}, content={line['content']}")
            
            # Add matching lines to target lines
            if not args.line:
                args.line = []
            for line in matching_lines:
                if line['old_line'] > 0:
                    args.line.append(line['old_line'])
                if line['new_line'] > 0:
                    args.line.append(line['new_line'])
        else:
            print("No matching lines found")
    
    # Display line information
    display_line_info(line_info, args.line)
    
    # Special instructions for target lines
    if args.line:
        print("\n=== TARGET LINE INSTRUCTIONS ===")
        for line_num in args.line:
            if line_num in line_info:
                info = line_info[line_num]
                if info['type'] == 'deleted':
                    print(f"Line {line_num} is a DELETED line. Use old_line={line_num} parameter in GitLab API")
                elif info['type'] == 'added':
                    print(f"Line {line_num} is an ADDED line. Use new_line={line_num} parameter in GitLab API")
                else:
                    print(f"Line {line_num} is a CONTEXT line. Use either old_line={info['old_line']} or new_line={info['new_line']} parameter")
            else:
                # Check if it's referenced by old_line
                found = False
                for info in line_info.values():
                    if info['old_line'] == line_num:
                        found = True
                        print(f"Line {line_num} is an OLD line number (deleted). Use old_line={line_num} parameter in GitLab API")
                        break
                
                if not found:
                    print(f"Line {line_num} not found in the diff")

if __name__ == "__main__":
    main()
