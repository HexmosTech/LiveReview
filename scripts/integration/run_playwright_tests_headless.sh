#!/bin/bash
# LiveReview Playwright Test Runner (Headless)

cd /home/shrsv/bin/LiveReview/scripts/integration

echo "Running Playwright tests in headless mode..."
echo "============================================="

# Run in headless mode (no browser visible)
npx playwright test livereview.spec.js

echo ""
echo "Test run completed!"