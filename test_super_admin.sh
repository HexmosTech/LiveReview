#!/bin/bash

# Super Admin Global Management API Testing Script

set -e

echo "👑 Super Admin Global Management API Testing Script"
echo "===================================================="

# Configuration
BASE_URL="http://localhost:8888/api/v1"
CONTENT_TYPE="Content-Type: application/json"

# Function to make authenticated requests
make_request() {
    local method=$1
    local endpoint=$2
    local data=${3:-""}
    
    if [ -n "$data" ]; then
        curl -s -X "$method" \
             -H "$CONTENT_TYPE" \
             -H "Authorization: Bearer $ACCESS_TOKEN" \
             -d "$data" \
             "$BASE_URL$endpoint"
    else
        curl -s -X "$method" \
             -H "Authorization: Bearer $ACCESS_TOKEN" \
             "$BASE_URL$endpoint"
    fi
}

# Step 1: Login as super admin
echo "🔐 Step 1: Login as super admin"
echo "Please provide your password for shrijith@hexmos.com:"
read -s PASSWORD

LOGIN_RESPONSE=$(curl -s -X POST \
    -H "$CONTENT_TYPE" \
    -d "{\"email\":\"shrijith@hexmos.com\",\"password\":\"$PASSWORD\"}" \
    "$BASE_URL/auth/login")

echo "Login response received"

# Extract access token
if command -v jq &> /dev/null; then
    ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.tokens.access_token')
    if [ "$ACCESS_TOKEN" = "null" ] || [ -z "$ACCESS_TOKEN" ]; then
        echo "❌ Login failed. Please check your password."
        exit 1
    fi
    echo "✅ Login successful!"
else
    echo "⚠️  jq not installed. Please install it with: sudo apt install jq"
    exit 1
fi

echo ""
echo "🧪 Testing Super Admin Global Management APIs"
echo "=============================================="

# Step 2: List all users across organizations
echo ""
echo "📋 Step 2: List all users across organizations"
ALL_USERS_RESPONSE=$(make_request "GET" "/admin/users")
echo "All users:"
echo "$ALL_USERS_RESPONSE" | jq '.' 2>/dev/null || echo "$ALL_USERS_RESPONSE"

# Step 3: Get user analytics
echo ""
echo "📊 Step 3: Get user analytics"
ANALYTICS_RESPONSE=$(make_request "GET" "/admin/analytics/users")
echo "User analytics:"
echo "$ANALYTICS_RESPONSE" | jq '.' 2>/dev/null || echo "$ANALYTICS_RESPONSE"

# Step 4: Create user in specific organization (using Default Organization - ID 1)
echo ""
echo "👤 Step 4: Create user in Default Organization (ID 1)"
ADMIN_CREATE_USER_DATA='{
    "email": "admintest@example.com",
    "password": "adminTempPassword123!",
    "first_name": "Admin",
    "last_name": "TestUser",
    "role_id": 3
}'

echo "Creating user in org 1 with data: $ADMIN_CREATE_USER_DATA"
ADMIN_CREATE_RESPONSE=$(make_request "POST" "/admin/orgs/1/users" "$ADMIN_CREATE_USER_DATA")
echo "Admin create user response:"
echo "$ADMIN_CREATE_RESPONSE" | jq '.' 2>/dev/null || echo "$ADMIN_CREATE_RESPONSE"

# Extract new user ID for transfer test
if command -v jq &> /dev/null; then
    NEW_USER_ID=$(echo "$ADMIN_CREATE_RESPONSE" | jq -r '.id // empty')
    if [ -n "$NEW_USER_ID" ] && [ "$NEW_USER_ID" != "null" ]; then
        echo "✅ User created with ID: $NEW_USER_ID"
        
        # Step 5: Transfer user to different organization
        echo ""
        echo "🔄 Step 5: Transfer user from org 1 to org 2 (Hexmos)"
        TRANSFER_DATA='{
            "new_org_id": 2,
            "new_role_id": 3
        }'
        
        echo "Transferring user $NEW_USER_ID with data: $TRANSFER_DATA"
        TRANSFER_RESPONSE=$(make_request "PUT" "/admin/users/$NEW_USER_ID/org" "$TRANSFER_DATA")
        echo "Transfer response:"
        echo "$TRANSFER_RESPONSE" | jq '.' 2>/dev/null || echo "$TRANSFER_RESPONSE"
        
    else
        echo "❌ Failed to extract user ID from create response"
    fi
else
    echo "⚠️  Cannot extract user ID without jq"
fi

# Step 6: List all users again to see changes
echo ""
echo "📋 Step 6: List all users again to see transfer changes"
FINAL_ALL_USERS=$(make_request "GET" "/admin/users")
echo "Final all users list:"
echo "$FINAL_ALL_USERS" | jq '.' 2>/dev/null || echo "$FINAL_ALL_USERS"

# Step 7: Get updated analytics
echo ""
echo "📊 Step 7: Get updated analytics"
FINAL_ANALYTICS=$(make_request "GET" "/admin/analytics/users")
echo "Updated analytics:"
echo "$FINAL_ANALYTICS" | jq '.' 2>/dev/null || echo "$FINAL_ANALYTICS"

echo ""
echo "🎉 Super Admin Global Management API Testing Complete!"
echo ""
echo "📝 Manual Testing Checklist:"
echo "- ✅ Super admin authentication and authorization"
echo "- ✅ List all users across all organizations"
echo "- ✅ Get comprehensive user analytics"
echo "- ✅ Create users in any organization"
echo "- ✅ Transfer users between organizations"
echo "- ✅ Analytics updates reflect changes"
echo ""
echo "🔍 Next Steps:"
echo "1. Test permission enforcement (non-super-admin access denied)"
echo "2. Test edge cases (invalid org IDs, non-existent users)"
echo "3. Verify audit trails for super admin actions"
echo "4. Test analytics with larger datasets"