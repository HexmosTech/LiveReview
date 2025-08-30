#!/bin/bash

# User Management API Testing Script
# This script tests the org-scoped user management APIs

set -e

echo "🚀 User Management API Testing Script"
echo "======================================"

# Configuration
BASE_URL="http://localhost:8888/api/v1"
CONTENT_TYPE="Content-Type: application/json"

# Test data
ORG_ID=2  # Hexmos organization
USER_EMAIL="shrijith@hexmos.com"
USER_PASSWORD="your_password_here"  # You'll need to provide this

echo ""
echo "📋 Current Database State:"
echo "- Organizations: 1=Default Organization, 2=Hexmos"
echo "- Roles: 1=super_admin, 2=owner, 3=member"
echo "- Existing user: shrijith@hexmos.com (super_admin in org 2)"
echo ""

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

# Step 1: Login to get access token
echo "🔐 Step 1: Login to get access token"
echo "Please provide your password for shrijith@hexmos.com:"
read -s PASSWORD

LOGIN_RESPONSE=$(curl -s -X POST \
    -H "$CONTENT_TYPE" \
    -d "{\"email\":\"$USER_EMAIL\",\"password\":\"$PASSWORD\"}" \
    "$BASE_URL/auth/login")

echo "Login response: $LOGIN_RESPONSE"

# Extract access token (you might need to install jq: sudo apt install jq)
if command -v jq &> /dev/null; then
    ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.tokens.access_token')
    if [ "$ACCESS_TOKEN" = "null" ] || [ -z "$ACCESS_TOKEN" ]; then
        echo "❌ Login failed. Please check your password."
        exit 1
    fi
    echo "✅ Login successful!"
else
    echo "⚠️  jq not installed. Please install it or manually extract the access_token from the login response above."
    echo "Run: sudo apt install jq"
    exit 1
fi

echo ""
echo "🧪 Testing User Management APIs"
echo "================================"

# Step 2: List users in organization
echo ""
echo "📋 Step 2: List users in organization $ORG_ID"
USERS_RESPONSE=$(make_request "GET" "/orgs/$ORG_ID/users")
echo "Users in org $ORG_ID:"
echo "$USERS_RESPONSE" | jq '.' 2>/dev/null || echo "$USERS_RESPONSE"

# Step 3: Create a new user
echo ""
echo "👤 Step 3: Create a new test user"
NEW_USER_DATA='{
    "email": "testuser@hexmos.com",
    "password": "tempPassword123!",
    "first_name": "Test",
    "last_name": "User",
    "role_id": 3
}'

echo "Creating user with data: $NEW_USER_DATA"
CREATE_RESPONSE=$(make_request "POST" "/orgs/$ORG_ID/users" "$NEW_USER_DATA")
echo "Create user response:"
echo "$CREATE_RESPONSE" | jq '.' 2>/dev/null || echo "$CREATE_RESPONSE"

# Extract new user ID for further testing
if command -v jq &> /dev/null; then
    NEW_USER_ID=$(echo "$CREATE_RESPONSE" | jq -r '.id // empty')
    if [ -n "$NEW_USER_ID" ] && [ "$NEW_USER_ID" != "null" ]; then
        echo "✅ User created with ID: $NEW_USER_ID"
        
        # Step 4: Get specific user details
        echo ""
        echo "🔍 Step 4: Get user details for user $NEW_USER_ID"
        USER_DETAILS=$(make_request "GET" "/orgs/$ORG_ID/users/$NEW_USER_ID")
        echo "User details:"
        echo "$USER_DETAILS" | jq '.' 2>/dev/null || echo "$USER_DETAILS"
        
        # Step 5: Update user
        echo ""
        echo "✏️  Step 5: Update user details"
        UPDATE_DATA='{"first_name": "UpdatedTest", "last_name": "UpdatedUser"}'
        UPDATE_RESPONSE=$(make_request "PUT" "/orgs/$ORG_ID/users/$NEW_USER_ID" "$UPDATE_DATA")
        echo "Update response:"
        echo "$UPDATE_RESPONSE" | jq '.' 2>/dev/null || echo "$UPDATE_RESPONSE"
        
        # Step 6: Change user role
        echo ""
        echo "🔄 Step 6: Change user role"
        ROLE_CHANGE_DATA='{"role_id": 2}'  # Change to owner
        ROLE_RESPONSE=$(make_request "PUT" "/orgs/$ORG_ID/users/$NEW_USER_ID/role" "$ROLE_CHANGE_DATA")
        echo "Role change response:"
        echo "$ROLE_RESPONSE" | jq '.' 2>/dev/null || echo "$ROLE_RESPONSE"
        
        # Step 7: Get audit log
        echo ""
        echo "📜 Step 7: Get user audit log"
        AUDIT_RESPONSE=$(make_request "GET" "/orgs/$ORG_ID/users/$NEW_USER_ID/audit-log")
        echo "Audit log:"
        echo "$AUDIT_RESPONSE" | jq '.' 2>/dev/null || echo "$AUDIT_RESPONSE"
        
        # Step 8: Force password reset
        echo ""
        echo "🔒 Step 8: Force password reset"
        RESET_RESPONSE=$(make_request "POST" "/orgs/$ORG_ID/users/$NEW_USER_ID/force-password-reset")
        echo "Password reset response:"
        echo "$RESET_RESPONSE" | jq '.' 2>/dev/null || echo "$RESET_RESPONSE"
        
    else
        echo "❌ Failed to extract user ID from create response"
    fi
else
    echo "⚠️  Cannot extract user ID without jq"
fi

# Step 9: List users again to see changes
echo ""
echo "📋 Step 9: List users again to see all changes"
FINAL_USERS=$(make_request "GET" "/orgs/$ORG_ID/users")
echo "Final user list:"
echo "$FINAL_USERS" | jq '.' 2>/dev/null || echo "$FINAL_USERS"

echo ""
echo "🎉 User Management API Testing Complete!"
echo ""
echo "📝 Manual Testing Checklist:"
echo "- ✅ Login and token authentication"
echo "- ✅ List users in organization" 
echo "- ✅ Create new user with temporary password"
echo "- ✅ Get specific user details"
echo "- ✅ Update user profile information"
echo "- ✅ Change user roles"
echo "- ✅ View audit log for user actions"
echo "- ✅ Force password reset"
echo ""
echo "🔍 Next Steps:"
echo "1. Try logging in as the new test user with: testuser@hexmos.com / tempPassword123!"
echo "2. Verify the user is forced to change password on first login"
echo "3. Test permission enforcement (member vs owner vs super_admin)"
echo "4. Test org boundary enforcement (try accessing users from different org)"