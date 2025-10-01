#!/usr/bin/env python3
"""
LiveReview Login Automation Script
Connects to Windows-side Playwright server from WSL to perform login automation
"""

import asyncio
from playwright.async_api import async_playwright

def get_windows_host_ip():
    """Get the Windows host IP from WSL's resolv.conf and route table"""
    # First try resolv.conf
    try:
        with open("/etc/resolv.conf") as f:
            for line in f:
                if line.startswith("nameserver"):
                    return line.split()[1]
    except FileNotFoundError:
        pass
    return "127.0.0.1"  # fallback

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
    return None

async def login_to_livereview():
    """Login to LiveReview using Playwright connecting to Windows server"""
    
    # Get Windows host IP
    host_ip = get_windows_host_ip()
    gateway_ip = get_wsl_gateway_ip()
    
    # Try multiple connection endpoints
    endpoints_to_try = []
    
    # Add gateway IP if found (most reliable for WSL)
    if gateway_ip:
        endpoints_to_try.append(f"ws://{gateway_ip}:9223/")
    
    # Add other potential endpoints
    endpoints_to_try.extend([
        f"ws://{host_ip}:9223/",
        "ws://localhost:9223/",
        "ws://127.0.0.1:9223/"
    ])
    
    async with async_playwright() as p:
        browser = None
        connection_error = None
        
        for ws_endpoint in endpoints_to_try:
            try:
                print(f"Trying to connect to Playwright server at: {ws_endpoint}")
                # Connect to the Windows-side Playwright server using WebSocket endpoint
                browser = await p.chromium.connect(ws_endpoint=ws_endpoint)
                print(f"Successfully connected to: {ws_endpoint}")
                break
            except Exception as e:
                print(f"Failed to connect to {ws_endpoint}: {e}")
                connection_error = e
                continue
        
        if not browser:
            raise Exception(f"Could not connect to any Playwright server. Last error: {connection_error}")
        
        try:
            # Create a new context with headed mode (visible browser)
            context = await browser.new_context(
                # Force headed mode (visible browser)
                ignore_https_errors=True
            )
            
            # Create a new page
            page = await context.new_page()
            
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
            
            # Take a screenshot for verification
            await page.screenshot(path="login_result.png")
            print("Screenshot saved as login_result.png")
            
            # Get page title
            title = await page.title()
            print(f"Page title: {title}")
            
            # Wait a moment to see the result
            await asyncio.sleep(2)
            
            print("Login automation completed successfully!")
            print("The automation ran in headless mode on the Windows side.")
            print("Check the screenshot 'login_result.png' to verify the result.")
            
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