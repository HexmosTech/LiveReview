import sqlite3


def get_user(conn, user_id):
    """Fetch a user record by id."""
    cursor = conn.cursor()
    cursor.execute("SELECT id, name, email FROM users WHERE id = ?", (user_id,))
    return cursor.fetchone()
