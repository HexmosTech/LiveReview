#!/bin/bash
# LiveReview Playwright Test Runner

cd /home/shrsv/bin/LiveReview/scripts/integration

echo "Running Playwright tests..."
echo "=========================="

# Run in headed mode (with browser visible)
npx playwright test livereview.spec.js --headed

echo ""
echo "Test run completed!"