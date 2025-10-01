import pytest
import os

def find_edge_executable():
    """Find Microsoft Edge executable path in WSL"""
    possible_paths = [
        "/usr/bin/microsoft-edge-dev",
        "/usr/bin/microsoft-edge",
        "/usr/bin/microsoft-edge-stable",
        "/usr/bin/microsoft-edge-beta",
        "/opt/microsoft/msedge/msedge",
        "/opt/microsoft/msedge-dev/msedge",
    ]
    
    for path in possible_paths:
        if os.path.exists(path):
            return path
    
    return None

@pytest.fixture(scope="session")
def browser_type_launch_args():
    """Configure browser launch arguments for pytest-playwright"""
    edge_path = find_edge_executable()
    if edge_path:
        return {
            "executable_path": edge_path,
            "headless": False
        }
    else:
        return {
            "headless": False
        }

def test_example_site(page):
    """Test that we can navigate to example.com"""
    page.goto("https://example.com")
    assert page.title() == "Example Domain"

def test_livereview_site_loads(page):
    """Test that LiveReview site loads"""
    page.goto("https://livereview.hexmos.site/")
    # Just check that the page loads and has the expected title
    assert "LiveReview" in page.title()

def test_livereview_login(page):
    """Test LiveReview login process"""
    page.goto("https://livereview.hexmos.site/")
    
    # Fill in login credentials
    page.fill('input[id="email-address"]', "general@hexmos.com")
    page.fill('input[id="password"]', "MegaGeneral@123")
    
    # Click submit
    page.click('button[type="submit"]')
    
    # Wait for navigation and verify login
    page.wait_for_load_state("networkidle")
    assert page.url != "https://livereview.hexmos.site/" or "dashboard" in page.url