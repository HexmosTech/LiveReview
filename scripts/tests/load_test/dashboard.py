import os
import sqlite3
import json
import pandas as pd
import streamlit as st

# Set page configuration
st.set_page_config(
    page_title="LiveReview Load Test Dashboard",
    layout="wide",
    initial_sidebar_state="expanded"
)

# Dark theme CSS tweaks
st.markdown("""
<style>
    .reportview-container {
        background-color: #0e1117;
    }
    .stMetric {
        background-color: #1f2937;
        padding: 15px;
        border-radius: 10px;
        border: 1px solid #374151;
        box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
    }
    .stButton>button {
        border-radius: 8px;
    }
</style>
""", unsafe_allow_html=True)

# Find database path
script_dir = os.path.dirname(os.path.realpath(__file__))
DB_PATH = os.path.join(script_dir, "load_test.db")

def get_connection():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

# State Management for Navigation
if 'page' not in st.session_state:
    st.session_state.page = 'tests_list'
if 'selected_test_id' not in st.session_state:
    st.session_state.selected_test_id = None
if 'selected_test_name' not in st.session_state:
    st.session_state.selected_test_name = None
if 'selected_review_id' not in st.session_state:
    st.session_state.selected_review_id = None

# Helpers for page navigation
def navigate_to(page, **kwargs):
    st.session_state.page = page
    for k, v in kwargs.items():
        st.session_state[k] = v
    st.rerun()

# ----------------- PAGE 1: TESTS LIST -----------------
def render_tests_list():
    st.title("LiveReview Load Tests")
    
    if not os.path.exists(DB_PATH):
        st.warning("No load test database found yet. Run a load test first to generate data.")
        return

    conn = get_connection()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT id, test_name, total_time, parallel_reviews_count, success_reviews, failed_reviews, created_at 
        FROM tests 
        ORDER BY id DESC
    """)
    rows = cursor.fetchall()
    conn.close()

    if not rows:
        st.info("The database is empty. Run a load test to see results.")
        return

    # Convert to Pandas DataFrame for calculations
    df = pd.DataFrame([dict(r) for r in rows])
    
    # Render overall stats
    total_runs = len(rows)
    total_reviews = sum(r['parallel_reviews_count'] for r in rows)
    total_success = sum(r['success_reviews'] for r in rows)
    total_failed = sum(r['failed_reviews'] for r in rows)
    success_rate = (total_success / total_reviews * 100) if total_reviews > 0 else 0
    
    col1, col2, col3, col4 = st.columns(4)
    col1.metric("Total Runs", f"{total_runs}")
    col2.metric("Total Reviews Submitted", f"{total_reviews}")
    col3.metric("Successful Reviews", f"{total_success}")
    col4.metric("Overall Success Rate", f"{success_rate:.1f}%")
    
    st.write("---")
    
    # Header row
    cols = st.columns([1, 3, 2, 2, 2, 2, 2])
    cols[0].markdown("**ID**")
    cols[1].markdown("**Test Run Name**")
    cols[2].markdown("**Total Duration**")
    cols[3].markdown("**Concurrently Enqueued**")
    cols[4].markdown("**Success count**")
    cols[5].markdown("**Failure count**")
    cols[6].markdown("**Action**")
    
    st.write("---")
    
    # Custom interactive table with select buttons
    for index, row in df.iterrows():
        cols = st.columns([1, 3, 2, 2, 2, 2, 2])
        cols[0].write(f"#{row['id']}")
        cols[1].write(f"**{row['test_name']}**")
        
        duration = row['total_time']
        dur_str = f"{duration:.2f}s" if duration < 60 else f"{(duration/60):.2f}m"
        cols[2].write(dur_str)
        
        cols[3].write(f"{row['parallel_reviews_count']} reviews")
        cols[4].write(f"{row['success_reviews']}")
        cols[5].write(f"{row['failed_reviews']}")
        
        # Details button
        if cols[6].button("Open Run", key=f"btn_run_{row['id']}"):
            navigate_to('reviews_list', selected_test_id=row['id'], selected_test_name=row['test_name'])

# ----------------- PAGE 2: REVIEWS UNDER TEST -----------------
def render_reviews_list():
    test_id = st.session_state.selected_test_id
    test_name = st.session_state.selected_test_name
    
    col1, col2 = st.columns([8, 2])
    col1.title(f"Run: {test_name}")
    if col2.button("← Back to Runs", type="secondary"):
        navigate_to('tests_list')
        
    conn = get_connection()
    cursor = conn.cursor()
    
    # Fetch test details
    cursor.execute("SELECT * FROM tests WHERE id = ?", (test_id,))
    test_details = cursor.fetchone()
    
    # Fetch reviews
    cursor.execute("""
        SELECT review_id, status, time_taken, avg_poll_latency, first_comment_offset 
        FROM reviews 
        WHERE test_id = ?
        ORDER BY CAST(review_id AS INTEGER) ASC, review_id ASC
    """, (test_id,))
    rows = cursor.fetchall()
    conn.close()

    if not test_details:
        st.error("Test run not found.")
        return

    # Render metrics for the specific run
    col1, col2, col3, col4 = st.columns(4)
    total_time_str = f"{test_details['total_time']:.2f}s" if test_details['total_time'] < 60 else f"{(test_details['total_time']/60):.2f}m"
    col1.metric("Run Time", total_time_str)
    col2.metric("Total Submitted", f"{test_details['parallel_reviews_count']}")
    col3.metric("Success Reviews", f"{test_details['success_reviews']}")
    col4.metric("Failed Reviews", f"{test_details['failed_reviews']}")

    if not rows:
        st.warning("No reviews found for this run.")
        return

    # Convert to Pandas DataFrame
    df_reviews = pd.DataFrame([dict(r) for r in rows])
    # Ensure review_id is numeric for correct sorting and X-axis mapping
    df_reviews['review_id_numeric'] = pd.to_numeric(df_reviews['review_id'], errors='coerce')
    df_reviews = df_reviews.sort_values(by='review_id_numeric')
    
    # Prepare chart data
    chart_data = pd.DataFrame({
        'Review ID': df_reviews['review_id_numeric'],
        'First Comment Offset (s)': df_reviews['first_comment_offset'],
        'Total Duration (s)': df_reviews['time_taken']
    })
    
    st.subheader("Queue Latency & Processing Times (Step Chart)")
    
    # Toggle and controls for showing static labels (great for screenshots)
    col_t1, col_t2 = st.columns(2)
    show_labels = col_t1.checkbox("Show static values on chart (for screenshots)", value=True)
    
    # Melt dataframe for Altair plotting
    df_melted = chart_data.melt(id_vars='Review ID', var_name='Metric', value_name='Time (s)')
    
    import altair as alt
    
    # Base chart encoding
    base = alt.Chart(df_melted).encode(
        x=alt.X('Review ID:Q', title='Review ID', scale=alt.Scale(zero=False)),
        y=alt.Y('Time (s):Q', title='Time (seconds)'),
        color=alt.Color('Metric:N', legend=alt.Legend(orient='bottom', title=None))
    )
    
    # Render lines & points
    lines = base.mark_line(strokeWidth=2)
    points = base.mark_circle(size=40)
    chart = lines + points
    
    if show_labels:
        label_step = col_t2.slider("Label frequency (show value every Nth point)", min_value=1, max_value=10, value=2)
        
        # Sub-slice data to avoid cluttering the visual
        df_reviews_labels = df_reviews.iloc[::label_step]
        chart_data_labels = pd.DataFrame({
            'Review ID': df_reviews_labels['review_id_numeric'],
            'First Comment Offset (s)': df_reviews_labels['first_comment_offset'],
            'Total Duration (s)': df_reviews_labels['time_taken']
        })
        df_labels = chart_data_labels.melt(id_vars='Review ID', var_name='Metric', value_name='Time (s)')
        
        # Render static text label layer
        labels = alt.Chart(df_labels).mark_text(
            align='center',
            baseline='bottom',
            dy=-10,
            fontSize=10,
            fontWeight='bold'
        ).encode(
            x='Review ID:Q',
            y='Time (s):Q',
            text=alt.Text('Time (s):Q', format='.1f'),
            color='Metric:N'
        )
        chart = chart + labels
        
    chart = chart.properties(height=400).interactive()
    
    st.altair_chart(chart, use_container_width=True)
    
    st.write("---")

    # Header row
    cols = st.columns([2, 2, 2, 2, 2, 2])
    cols[0].markdown("**Review ID**")
    cols[1].markdown("**Status**")
    cols[2].markdown("**Time Taken**")
    cols[3].markdown("**Avg Poll Latency**")
    cols[4].markdown("**First Comment Offset**")
    cols[5].markdown("**Action**")
    
    st.write("---")
    
    # Display list of reviews
    for r in rows:
        cols = st.columns([2, 2, 2, 2, 2, 2])
        cols[0].write(f"**#{r['review_id']}**")
        status_text = r['status'].capitalize()
        cols[1].write(status_text)
        
        duration = r['time_taken']
        dur_str = f"{duration:.2f}s" if duration < 60 else f"{(duration/60):.2f}m"
        cols[2].write(dur_str)
        
        cols[3].write(f"{r['avg_poll_latency']:.3f}s")
        
        fc = r['first_comment_offset']
        fc_str = f"{fc:.2f}s" if fc is not None else "N/A"
        cols[4].write(fc_str)
        
        if cols[5].button("View Details", key=f"btn_rev_{r['review_id']}"):
            navigate_to('review_details', selected_review_id=r['review_id'])

# ----------------- PAGE 3: REVIEW DETAILS -----------------
def render_review_details():
    review_id = st.session_state.selected_review_id
    test_id = st.session_state.selected_test_id
    test_name = st.session_state.selected_test_name
    
    col1, col2 = st.columns([8, 2])
    col1.title(f"Review Details: #{review_id}")
    if col2.button("← Back to Reviews", type="secondary"):
        navigate_to('reviews_list', selected_test_id=test_id, selected_test_name=test_name)
        
    conn = get_connection()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT status, time_taken, avg_poll_latency, first_comment_offset, logs 
        FROM reviews 
        WHERE test_id = ? AND review_id = ?
    """, (test_id, review_id))
    row = cursor.fetchone()
    conn.close()

    if not row:
        st.error("Review not found.")
        return

    # Render review metrics
    col1, col2, col3, col4 = st.columns(4)
    col1.metric("Status", row['status'].capitalize())
    
    duration = row['time_taken']
    dur_str = f"{duration:.2f}s" if duration < 60 else f"{(duration/60):.2f}m"
    col2.metric("Time Taken", dur_str)
    
    col3.metric("Avg Poll Latency", f"{row['avg_poll_latency']:.3f}s")
    
    fc = row['first_comment_offset']
    fc_str = f"{fc:.2f}s" if fc is not None else "N/A"
    col4.metric("First Comment Offset", fc_str)

    st.write("---")
    
    try:
        events = json.loads(row['logs'])
    except Exception:
        events = []

    if not events:
        st.info("No events recorded for this review.")
        return

    # Reconstruct formatted log blocks
    log_blocks = []
    for i, event in enumerate(events):
        event_type = event.get("type", "").upper()
        event_level = (event.get("level") or "info").upper()
        event_time = event.get("time", "")
        data = event.get("data") or {}

        if isinstance(data, str):
            try:
                data = json.loads(data)
            except Exception:
                pass

        msg = data.get("message") or data.get("resultSummary") or ""
        
        details_dict = {k: v for k, v in data.items() if k not in ("message", "resultSummary") and v is not None}
        details_str = ""
        if details_dict:
            details_str = "\n  Details: " + json.dumps(details_dict, indent=2).replace("\n", "\n  ")

        header = f"[{i + 1}] {event_time} - {event_type} - {event_level}"
        log_block = f"{header}\n  {msg}{details_str}"
        log_blocks.append(log_block)

    # Search Box
    search_query = st.text_input("Search through logs", "").strip().lower()
    
    if search_query:
        filtered_blocks = [block for block in log_blocks if search_query in block.lower()]
    else:
        filtered_blocks = log_blocks

    st.subheader("Event Logs")
    if filtered_blocks:
        log_text = "\n\n".join(filtered_blocks)
        st.code(log_text, language="log")
    else:
        st.info("No matching log entries found.")

# Router
if st.session_state.page == 'tests_list':
    render_tests_list()
elif st.session_state.page == 'reviews_list':
    render_reviews_list()
elif st.session_state.page == 'review_details':
    render_review_details()
