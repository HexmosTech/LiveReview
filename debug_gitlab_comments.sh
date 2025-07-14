#!/bin/bash
# GitLab Line Comment Debug Suite
# This script provides a convenient way to run the various debugging tools

# Configure your GitLab token here or set it in your environment
GITLAB_TOKEN=${GITLAB_TOKEN:-"REDACTED_GITLAB_PAT_6"}
MR_URL="https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/403"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

function print_header() {
    echo -e "\n${BLUE}======================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}======================================================${NC}"
}

function show_usage() {
    echo "GitLab Line Comment Debug Suite"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  analyze    Analyze hunk logs for a specific file"
    echo "  test44     Test commenting on line 44 (added line)"
    echo "  test160    Test commenting on line 160 (deleted line)"
    echo "  testall    Test both problematic lines"
    echo "  toolkit    Run the comprehensive toolkit with specified options"
    echo ""
    echo "Examples:"
    echo "  $0 analyze gk_input_handler.go_hunks.log"
    echo "  $0 test44"
    echo "  $0 test160"
    echo "  $0 testall"
    echo "  $0 toolkit --test-problem-lines"
    echo ""
}

function check_python_deps() {
    print_header "Checking Python Dependencies"
    
    # Check if pip is installed
    if ! command -v pip &> /dev/null; then
        echo -e "${RED}Error: pip is not installed. Please install pip first.${NC}"
        exit 1
    fi
    
    # List of required packages
    packages=("requests" "colorama" "argparse")
    
    # Check and install missing packages
    for pkg in "${packages[@]}"; do
        if ! python3 -c "import $pkg" &> /dev/null; then
            echo -e "${YELLOW}Installing package: $pkg${NC}"
            pip install $pkg
        else
            echo -e "${GREEN}Package already installed: $pkg${NC}"
        fi
    done
}

# Make scripts executable
function ensure_executable() {
    chmod +x "$1"
}

case "$1" in
    analyze)
        check_python_deps
        if [ -z "$2" ]; then
            echo -e "${RED}Error: No hunk log file specified${NC}"
            echo "Usage: $0 analyze <hunk_log_file> [--line LINE_NUMBER]"
            exit 1
        fi
        
        hunk_file="$2"
        shift 2
        
        ensure_executable "./analyze_hunk_log.py"
        print_header "Analyzing Hunk Log: $hunk_file"
        
        # Check if the file exists in the hunk_format_logs directory
        if [ ! -f "$hunk_file" ] && [ -f "hunk_format_logs/$hunk_file" ]; then
            hunk_file="hunk_format_logs/$hunk_file"
        fi
        
        ./analyze_hunk_log.py "$hunk_file" "$@"
        ;;
        
    test44)
        check_python_deps
        ensure_executable "./test_line44.py"
        print_header "Testing Line Comment on Line 44 (Added Line)"
        ./test_line44.py
        ;;
        
    test160)
        check_python_deps
        ensure_executable "./test_line160.py"
        print_header "Testing Line Comment on Line 160 (Deleted Line)"
        ./test_line160.py
        ;;
        
    testall)
        check_python_deps
        ensure_executable "./test_line44.py"
        ensure_executable "./test_line160.py"
        
        print_header "Testing Line Comment on Line 44 (Added Line)"
        ./test_line44.py
        
        print_header "Testing Line Comment on Line 160 (Deleted Line)"
        ./test_line160.py
        ;;
        
    toolkit)
        check_python_deps
        ensure_executable "./gitlab_line_comment_toolkit.py"
        shift
        
        # If no arguments provided, show help
        if [ $# -eq 0 ]; then
            ./gitlab_line_comment_toolkit.py --help
            exit 0
        fi
        
        print_header "Running GitLab Line Comment Toolkit"
        ./gitlab_line_comment_toolkit.py --mr-url "$MR_URL" --token "$GITLAB_TOKEN" "$@"
        ;;
        
    *)
        show_usage
        ;;
esac
