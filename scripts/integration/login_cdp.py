#!/usr/bin/env python3
"""
LiveReview Login Automation Script using CDP
Connects to Chrome with remote debugging enabled on Windows
"""

import asyncio
from playwright.async_api import async_playwright

def get_wsl_gateway_ip():
    """Get the WSL gateway IP (Windows host) from route table"""
    import subprocess
    try:
        # Get the default gateway from ip route
        result = subprocess.run(['ip', 'route', 'show'], 
                              capture_output=True, text=True, check=True)
        for line in result.stdout.split('\n'):
            if line.startswith('default via'):
                # Extract gateway IP from "default via 192.168.128.1 dev eth0"
                parts = line.split()
                if len(parts) >= 3:
                    return parts[2]
    except (subprocess.CalledProcessError, FileNotFoundError, IndexError):
        pass
    return "192.168.128.1"  # fallback

async def login_to_livereview():
    """Login to LiveReview using CDP connection to Chrome"""
    
    gateway_ip = get_wsl_gateway_ip()
    cdp_endpoint = f"http://{gateway_ip}:9222"
    
    print(f"Connecting to Chrome via CDP at: {cdp_endpoint}")
    
    async with async_playwright() as p:
        try:
            # Connect to Chrome via CDP (Chrome DevTools Protocol)
            browser = await p.chromium.connect_over_cdp(cdp_endpoint)
            print(f"Successfully connected to Chrome via CDP")
            
            # Create a new page (this will be visible in the Chrome window)
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
            
            # Check if login was successful
            current_url = page.url
            print(f"Current URL after login: {current_url}")
            
            # Take a screenshot for verification
            await page.screenshot(path="login_result.png")
            print("Screenshot saved as login_result.png")
            
            # Get page title
            title = await page.title()
            print(f"Page title: {title}")
            
            # Wait a moment to see the result
            await asyncio.sleep(5)
            
            print("Login automation completed successfully!")
            print("You can see the browser window on Windows and continue using it manually.")
            
        except Exception as e:
            print(f"Error during login automation: {e}")
            raise
        finally:
            # Don't close the browser so user can continue using it
            print("Browser left open for manual use.")

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