#!/usr/bin/env python3
"""
LiveReview Login Automation Script
Uses Microsoft Edge within WSL with WSLg for visible browser automation
"""

import asyncio
import os
from playwright.async_api import async_playwright

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

async def login_to_livereview():
    """Login to LiveReview using Playwright connecting to Windows server"""
    
    # Find Edge executable
    edge_path = find_edge_executable()
    if not edge_path:
        raise Exception("Microsoft Edge not found in WSL. Please install Microsoft Edge.")
    
    print(f"Using Microsoft Edge at: {edge_path}")
    
    async with async_playwright() as p:
        browser = None
        
        try:
            # Launch Microsoft Edge in headed mode (visible browser)
            browser = await p.chromium.launch(
                executable_path=edge_path,
                headless=False  # This ensures the browser window is visible
            )
            
            print("Microsoft Edge launched successfully!")
            
            # Create a new page
            page = await browser.new_page()
            
            print("Navigating to LiveReview login page...")
            await page.goto("https://livereview.hexmos.site/")
            
            # Wait for the page to load
            await page.wait_for_load_state("networkidle")
            
            print("Filling in login credentials...")
            
            # Fill in the email address
            await page.fill('input[id="email-address"]', "general@hexmos.com")
            
            # Fill in the password
            await page.fill('input[id="password"]', "MegaGeneral@123")
            
            print("Submitting login form...")
            
            # Click the submit button
            await page.click('button[type="submit"]')
            
            # Wait for navigation after login
            await page.wait_for_load_state("networkidle")
            
            # Check if login was successful by looking for common post-login elements
            current_url = page.url
            print(f"Current URL after login: {current_url}")
            
            print("Waiting for 'Recent Activity' section to become visible...")
            
            try:
                # Wait for the "Recent Activity" heading to become visible (timeout after 15 seconds)
                await page.wait_for_selector('h3:has-text("Recent Activity")', timeout=15000)
                print("'Recent Activity' section detected in DOM...")
                
                # Wait a bit more for the content to fully load after the heading appears
                await asyncio.sleep(2)
                
                # Also wait for any loading indicators to disappear
                try:
                    await page.wait_for_selector('[data-testid="loading"], .loading, .spinner', state='hidden', timeout=5000)
                    print("Loading indicators hidden.")
                except:
                    print("No loading indicators found or they didn't hide, continuing...")
                
                # Take a screenshot after Recent Activity appears and content loads
                await page.screenshot(path="login_result.png")
                print("Screenshot saved as 'login_result.png'")
                
            except Exception as e:
                print(f"'Recent Activity' section did not appear within timeout: {e}")
                # Take a screenshot anyway to see what's on the page
                await page.screenshot(path="login_result.png")
                print("Screenshot saved as 'login_result.png' (with timeout)")
            
            # Get page title
            title = await page.title()
            print(f"Page title: {title}")
            
            # Wait a moment to see the result
            await asyncio.sleep(2)
            
            print("Login automation completed successfully!")
            
        except Exception as e:
            print(f"Error during login automation: {e}")
            raise
        finally:
            # Close the browser
            if browser:
                await browser.close()

async def main():
    """Main function to run the login automation"""
    try:
        await login_to_livereview()
    except Exception as e:
        print(f"Login automation failed: {e}")
        return 1  # Return error code
    
    return 0  # Success

if __name__ == "__main__":
    import sys
    exit_code = asyncio.run(main())
    sys.exit(exit_code)