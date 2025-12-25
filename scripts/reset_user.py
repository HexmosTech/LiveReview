#!/usr/bin/env python3
"""
Reset user onboarding state for testing purposes.
Clears CLI usage, reviews, connectors, and other onboarding-related data.
"""

import os
import sys
import psycopg2
from psycopg2.extras import RealDictCursor

# Hardcoded user email for testing
TARGET_EMAIL = "contortedexpression@gmail.com"

def get_db_connection():
    """Get database connection from .env.prod"""
    env_path = os.path.join(os.path.dirname(__file__), "..", ".env.prod")
    
    # Read DATABASE_URL from .env.prod
    db_url = None
    with open(env_path, 'r') as f:
        for line in f:
            if line.startswith('DATABASE_URL='):
                db_url = line.split('=', 1)[1].strip()
                break
    
    if not db_url:
        raise Exception("DATABASE_URL not found in .env.prod")
    
    # Parse postgres URL: postgres://user:pass@host:port/dbname?sslmode=disable
    # Format: postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]
    db_url = db_url.replace('postgres://', 'postgresql://')
    
    return psycopg2.connect(db_url)

def reset_user_onboarding(email):
    """Reset user's onboarding state"""
    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)
    
    try:
        # Get user info
        cursor.execute("SELECT id, email FROM users WHERE email = %s", (email,))
        user = cursor.fetchone()
        
        if not user:
            print(f"❌ User not found: {email}")
            return False
        
        user_id = user['id']
        print(f"✓ Found user: {user['email']} - ID: {user_id}")
        
        # Get user's organizations
        cursor.execute("""
            SELECT o.id, o.name
            FROM orgs o
            INNER JOIN user_roles ur ON o.id = ur.org_id
            WHERE ur.user_id = %s
        """, (user_id,))
        orgs = cursor.fetchall()
        
        if not orgs:
            print(f"⚠️  User has no organizations")
        else:
            print(f"✓ User belongs to {len(orgs)} organization(s):")
            for org in orgs:
                print(f"  - {org['name']} (ID: {org['id']})")
        
        # Start reset process
        print("\n🔄 Starting reset process...")
        
        # 1. Reset CLI usage timestamp
        cursor.execute("UPDATE users SET last_cli_used_at = NULL WHERE id = %s", (user_id,))
        print("✓ Cleared CLI usage timestamp")
        
        # 2. Delete reviews for user's organizations
        if orgs:
            org_ids = [org['id'] for org in orgs]
            cursor.execute("SELECT COUNT(*) as count FROM reviews WHERE org_id = ANY(%s)", (org_ids,))
            review_count = cursor.fetchone()['count']
            
            if review_count > 0:
                cursor.execute("DELETE FROM reviews WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {review_count} review(s)")
            else:
                print("✓ No reviews to delete")
        
        # 3. Delete AI connectors for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM ai_connectors 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            ai_count = cursor.fetchone()['count']
            
            if ai_count > 0:
                cursor.execute("DELETE FROM ai_connectors WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {ai_count} AI connector(s)")
            else:
                print("✓ No AI connectors to delete")
        
        # 4. Delete integration tokens (Git providers) for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM integration_tokens 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            token_count = cursor.fetchone()['count']
            
            if token_count > 0:
                cursor.execute("DELETE FROM integration_tokens WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {token_count} integration token(s)")
            else:
                print("✓ No integration tokens to delete")
        
        # 5. Delete API keys for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM api_keys 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            key_count = cursor.fetchone()['count']
            
            if key_count > 0:
                cursor.execute("DELETE FROM api_keys WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {key_count} API key(s)")
            else:
                print("✓ No API keys to delete")
        
        # 6. Delete recent activity for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM recent_activity 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            activity_count = cursor.fetchone()['count']
            
            if activity_count > 0:
                cursor.execute("DELETE FROM recent_activity WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {activity_count} activity record(s)")
            else:
                print("✓ No activity records to delete")
        
        # 6.5. Delete prompt_application_context for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM prompt_application_context 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            prompt_context_count = cursor.fetchone()['count']
            
            if prompt_context_count > 0:
                cursor.execute("DELETE FROM prompt_application_context WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {prompt_context_count} prompt context(s)")
            else:
                print("✓ No prompt contexts to delete")
        
        # 7. Delete user roles (membership in organizations)
        cursor.execute("SELECT COUNT(*) as count FROM user_roles WHERE user_id = %s", (user_id,))
        role_count = cursor.fetchone()['count']
        
        if role_count > 0:
            cursor.execute("DELETE FROM user_roles WHERE user_id = %s", (user_id,))
            print(f"✓ Deleted {role_count} user role(s)")
        else:
            print("✓ No user roles to delete")
        
        # 8. Delete auth tokens for the user
        cursor.execute("SELECT COUNT(*) as count FROM auth_tokens WHERE user_id = %s", (user_id,))
        token_count = cursor.fetchone()['count']
        
        if token_count > 0:
            cursor.execute("DELETE FROM auth_tokens WHERE user_id = %s", (user_id,))
            print(f"✓ Deleted {token_count} auth token(s)")
        else:
            print("✓ No auth tokens to delete")
        
        # 9. Delete license_log entries for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM license_log 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            license_log_count = cursor.fetchone()['count']
            
            if license_log_count > 0:
                cursor.execute("DELETE FROM license_log WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {license_log_count} license log(s)")
            else:
                print("✓ No license logs to delete")
        
        # 10. Delete subscriptions for user's organizations
        if orgs:
            cursor.execute("""
                SELECT COUNT(*) as count 
                FROM subscriptions 
                WHERE org_id = ANY(%s)
            """, (org_ids,))
            subscription_count = cursor.fetchone()['count']
            
            if subscription_count > 0:
                cursor.execute("DELETE FROM subscriptions WHERE org_id = ANY(%s)", (org_ids,))
                print(f"✓ Deleted {subscription_count} subscription(s)")
            else:
                print("✓ No subscriptions to delete")
        
        # 11. Delete the organizations if user was the only member
        
        if orgs:
            for org in orgs:
                cursor.execute("SELECT COUNT(*) as count FROM user_roles WHERE org_id = %s", (org['id'],))
                remaining_members = cursor.fetchone()['count']
                
                if remaining_members == 0:
                    cursor.execute("DELETE FROM orgs WHERE id = %s", (org['id'],))
                    print(f"✓ Deleted organization '{org['name']}' (no remaining members)")
        
        # 12. Delete the user
        cursor.execute("DELETE FROM users WHERE id = %s", (user_id,))
        print(f"✓ Deleted user account")
        
        # Commit all changes
        conn.commit()
        print("\n✅ User and all associated data deleted successfully!")
        print(f"🔗 User can now register again with a fresh account.")
        
        return True
        
    except Exception as e:
        conn.rollback()
        print(f"\n❌ Error during reset: {e}")
        import traceback
        traceback.print_exc()
        return False
        
    finally:
        cursor.close()
        conn.close()

def main():
    print(f"🔧 LiveReview User Deletion Tool")
    print(f"=" * 50)
    print(f"Target user: {TARGET_EMAIL}")
    print(f"=" * 50)
    print()
    
    # Confirm
    response = input(f"⚠️  This will PERMANENTLY DELETE {TARGET_EMAIL} and all associated data. Continue? [y/N]: ")
    if response.lower() != 'y':
        print("❌ Cancelled")
        return 1
    
    print()
    success = reset_user_onboarding(TARGET_EMAIL)
    
    return 0 if success else 1

if __name__ == "__main__":
    sys.exit(main())
