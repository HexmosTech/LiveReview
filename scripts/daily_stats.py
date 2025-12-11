import os
import psycopg2
from urllib.parse import urlparse
import datetime

def get_db_connection():
    """Establishes a connection to the database using the URL in this file."""
    db_url = "postgres://flyweightpostgres:REDACTED_DB_PASSWORD@REDACTED_DB_HOST:5432/livereview?sslmode=disable"
    
    result = urlparse(db_url)
    username = result.username
    password = result.password
    database = result.path[1:]
    hostname = result.hostname
    port = result.port
    
    conn = psycopg2.connect(
        dbname=database,
        user=username,
        password=password,
        host=hostname,
        port=port
    )
    return conn

def get_daily_stats():
    """Fetches daily statistics from the database."""
    conn = get_db_connection()
    cursor = conn.cursor()
    
    today = datetime.date.today()
    
    # 1. How many new AI connectors today
    cursor.execute("SELECT COUNT(*) FROM ai_connectors WHERE DATE(created_at) = %s", (today,))
    ai_connectors_count = cursor.fetchone()[0]
    
    # 2. How many new git connectors today (assuming integration_tokens for git connectors)
    cursor.execute("SELECT COUNT(*) FROM integration_tokens WHERE DATE(created_at) = %s", (today,))
    git_connectors_count = cursor.fetchone()[0]
    
    # 3. How many reviews created today
    cursor.execute("SELECT COUNT(*) FROM reviews WHERE DATE(created_at) = %s", (today,))
    reviews_count = cursor.fetchone()[0]

    # 4. How many new users created today
    cursor.execute("SELECT COUNT(*) FROM users WHERE DATE(created_at) = %s", (today,))
    new_users_count = cursor.fetchone()[0]
    
    cursor.close()
    conn.close()
    
    return {
        "date": today,
        "new_ai_connectors": ai_connectors_count,
        "new_git_connectors": git_connectors_count,
        "reviews_created": reviews_count,
        "new_users": new_users_count
    }

def print_report(stats):
    """Prints a formatted report of the daily stats."""
    print("--- Daily Stats Report ---")
    print(f"Date: {stats['date']}")
    print(f"New AI Connectors: {stats['new_ai_connectors']}")
    print(f"New Git Connectors: {stats['new_git_connectors']}")
    print(f"New Users Created: {stats['new_users']}")
    print(f"Reviews Created: {stats['reviews_created']}")
    print("--------------------------")

if __name__ == "__main__":
    try:
        # The script needs psycopg2 to run. Let's make sure it's installed.
        # You can install it by running: pip install psycopg2-binary
        stats = get_daily_stats()
        print_report(stats)
    except ImportError:
        print("psycopg2 is not installed. Please install it using: pip install psycopg2-binary")
    except Exception as e:
        print(f"An error occurred: {e}")
