#!/bin/bash

# Profile Management API Testing Script

set -e

echo "🔧 Profile Management API Testing Script"
echo "========================================="

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

# Step 1: Login to get access token
echo "🔐 Step 1: Login to get access token"
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
echo "🧪 Testing Profile Management APIs"
echo "==================================="

# Step 2: Get current profile
echo ""
echo "👤 Step 2: Get current user profile"
PROFILE_RESPONSE=$(make_request "GET" "/users/profile")
echo "Current profile:"
echo "$PROFILE_RESPONSE" | jq '.' 2>/dev/null || echo "$PROFILE_RESPONSE"

# Step 3: Update profile
echo ""
echo "✏️  Step 3: Update profile information"
UPDATE_DATA='{
    "first_name": "Shrijith",
    "last_name": "Venkatesh"
}'

echo "Updating profile with data: $UPDATE_DATA"
UPDATE_RESPONSE=$(make_request "PUT" "/users/profile" "$UPDATE_DATA")
echo "Update response:"
echo "$UPDATE_RESPONSE" | jq '.' 2>/dev/null || echo "$UPDATE_RESPONSE"

# Step 4: Get updated profile
echo ""
echo "🔍 Step 4: Get updated profile to verify changes"
UPDATED_PROFILE=$(make_request "GET" "/users/profile")
echo "Updated profile:"
echo "$UPDATED_PROFILE" | jq '.' 2>/dev/null || echo "$UPDATED_PROFILE"

# Step 5: Test password change (Optional - you can skip this)
echo ""
echo "🔒 Step 5: Test password change (Optional)"
echo "⚠️  WARNING: This will change your password!"
echo "Do you want to test password change? [y/N]"
read -n 1 -r REPLY
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Enter current password:"
    read -s CURRENT_PASS
    echo "Enter new password:"
    read -s NEW_PASS
    
    CHANGE_PASSWORD_DATA=$(cat <<EOF
{
    "current_password": "$CURRENT_PASS",
    "new_password": "$NEW_PASS"
}
EOF
)
    
    echo "Changing password..."
    PASSWORD_RESPONSE=$(make_request "PUT" "/users/password" "$CHANGE_PASSWORD_DATA")
    echo "Password change response:"
    echo "$PASSWORD_RESPONSE" | jq '.' 2>/dev/null || echo "$PASSWORD_RESPONSE"
else
    echo "Skipping password change test."
fi

echo ""
echo "🎉 Profile Management API Testing Complete!"
echo ""
echo "📝 Manual Testing Checklist:"
echo "- ✅ Login and token authentication"
echo "- ✅ Get user profile with organizations"
echo "- ✅ Update user profile information" 
echo "- ✅ Profile changes are persisted"
echo "- ✅ Password change functionality (if tested)"
echo ""
echo "🔍 Next Steps:"
echo "1. Test with different users (testuser@hexmos.com)"
echo "2. Test profile updates with different field combinations"
echo "3. Test email change validation"
echo "4. Test password change with incorrect current password"