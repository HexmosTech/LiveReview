#!/usr/bin/env python3
import os
import sys
import time
import random
from concurrent.futures import ThreadPoolExecutor, as_completed

# Add the script's directory to sys.path so modules can be imported
script_dir = os.path.dirname(os.path.realpath(__file__))
if script_dir not in sys.path:
    sys.path.insert(0, script_dir)

# Import modular helper functions
from config import get_api_key, get_api_url
from diff_utils import make_zip_from_test_repo, make_zip_from_samples
from api_client import submit_review, check_status, fetch_and_save_logs

def main():
    print("🚀 LiveReview Async Queue Concurrency Load Tester")
    print("================================================")
    
    # Generate random test-name
    adjectives = ["swift", "silent", "bold", "bright", "calm", "cool", "eager", "gentle", "happy", "jolly", "lucky", "proud", "brave", "witty", "frosty"]
    nouns = ["hawk", "river", "forest", "mountain", "whale", "panda", "tiger", "eagle", "cloud", "storm", "valley", "desert", "ocean", "star", "wolf"]
    test_name = f"{random.choice(adjectives)}-{random.choice(nouns)}-{random.randint(10, 99)}"
    
    logs_dir = os.path.join(script_dir, "logs", test_name)
    os.makedirs(logs_dir, exist_ok=True)
    
    print(f"[+] Started test run: {test_name}")
    print(f"[+] Logs will be saved under: {logs_dir}\n")
    
    # 1. Read API Key
    api_key = get_api_key()
    if not api_key:
        print("[-] API key could not be loaded from ~/.lrc.toml. Please authenticate first using 'lrc ui' or check the file.")
        sys.exit(1)
    print(f"[+] Loaded API key (prefix: {api_key[:8]}...)")

    api_url = get_api_url()
    num_jobs = 50
    if len(sys.argv) > 1:
        try:
            num_jobs = int(sys.argv[1])
        except ValueError:
            pass
            
    # 2. Package diff files (try external test repository first, fallback to samples)
    payload_b64 = make_zip_from_test_repo()
    if not payload_b64:
        print("[!] Falling back to sample diff files.")
        payload_b64 = make_zip_from_samples()
    if not payload_b64:
        print("[-] Failed to create ZIP payload. Exiting.")
        sys.exit(1)

    print(f"[+] Preparing to enqueue {num_jobs} reviews...")

    # 3. Submit jobs concurrently
    start_time = time.time()
    review_ids = []
    failed_submissions = 0
    review_start_times = {}
    review_durations = {}
    review_statuses = {}
    review_first_comment_offsets = {}

    print(f"[+] Submitting {num_jobs} reviews to {api_url}/api/v1/diff-review...")
    with ThreadPoolExecutor(max_workers=10) as executor:
        futures = [executor.submit(submit_review, api_url, api_key, payload_b64, i) for i in range(num_jobs)]
        for fut in as_completed(futures):
            res = fut.result()
            if res["status"] == "success":
                r_id = res["review_id"]
                review_ids.append(r_id)
                review_start_times[r_id] = time.time()
            else:
                failed_submissions += 1
                print(f" [-] Job {res['index']} failed to submit: {res.get('body') or res.get('error')}")

    submit_elapsed = time.time() - start_time
    print(f"[+] Submitted {len(review_ids)} reviews in {submit_elapsed:.2f} seconds ({failed_submissions} failed)")

    if not review_ids:
        print("[-] No reviews successfully submitted. Exiting.")
        sys.exit(1)

    # 4. Poll for completion
    print("\n⏳ Polling for all reviews to reach 'completed' status...")
    pending_ids = set(review_ids)
    poll_count = 0
    review_poll_latencies = {}
    
    while pending_ids:
        poll_count += 1
        if poll_count == 1 or poll_count % 5 == 0:
            success_count = sum(1 for status in review_statuses.values() if status == "completed")
            failed_count = sum(1 for status in review_statuses.values() if status == "failed")
            print(f"  • Progress: {len(pending_ids)} pending, {success_count} completed, {failed_count} failed")

        completed_this_round = []
        
        # Check status of pending reviews in parallel
        with ThreadPoolExecutor(max_workers=10) as executor:
            status_futures = {executor.submit(check_status, api_url, api_key, r_id): r_id for r_id in pending_ids}
            for fut in as_completed(status_futures):
                r_id = status_futures[fut]
                status, latency = fut.result()
                if r_id not in review_poll_latencies:
                    review_poll_latencies[r_id] = []
                review_poll_latencies[r_id].append(latency)
                if status in ("completed", "failed"):
                    completed_this_round.append((r_id, status))
        
        for r_id, status in completed_this_round:
            pending_ids.remove(r_id)
            duration = time.time() - review_start_times[r_id]
            review_durations[r_id] = duration
            review_statuses[r_id] = status
            print(f"  [✓] Review {r_id} finished with status: {status} ({len(pending_ids)} remaining)")
            fc_offset = fetch_and_save_logs(api_url, api_key, r_id, logs_dir)
            review_first_comment_offsets[r_id] = fc_offset
            
        if pending_ids:
            time.sleep(1.0)

    def format_duration(seconds):
        if seconds < 60.0:
            return f"{seconds:.2f}s"
        return f"{(seconds / 60.0):.2f}m"

    total_elapsed = time.time() - start_time
    success_count = sum(1 for status in review_statuses.values() if status == "completed")
    failed_count = sum(1 for status in review_statuses.values() if status == "failed")
    
    summary_lines = [
        f"Test Name: {test_name}",
        f"Total Time: {format_duration(total_elapsed)}",
        f"Parallel Reviews Count: {num_jobs}",
        f"Success Reviews: {success_count}",
        f"Failed Reviews: {failed_count}",
        "",
        "Review ID    | Status       | Time Taken | Avg Poll Latency | First Comment",
        "-------------|--------------|------------|------------------|---------------"
    ]
    
    for r_id in sorted(review_ids, key=lambda x: int(x) if x.isdigit() else x):
        status = review_statuses.get(r_id, "unknown")
        duration = review_durations.get(r_id, 0.0)
        latencies = review_poll_latencies.get(r_id, [])
        avg_poll = sum(latencies) / len(latencies) if latencies else 0.0
        avg_poll_str = f"{avg_poll:.3f}s"
        fc_offset = review_first_comment_offsets.get(r_id)
        fc_offset_str = format_duration(fc_offset) if fc_offset is not None else "N/A"
        summary_lines.append(f"{r_id:<12} | {status:<12} | {format_duration(duration):<10} | {avg_poll_str:<16} | {fc_offset_str}")
        
    summary_content = "\n".join(summary_lines)
    summary_file_path = os.path.join(logs_dir, "summary.txt")
    with open(summary_file_path, "w", encoding="utf-8") as f:
        f.write(summary_content)
    
    print("\n================================================")
    print(f"🏆 LOAD TEST COMPLETED")
    print(f"  • Total Reviews: {len(review_ids)}")
    print(f"  • Total Time:    {format_duration(total_elapsed)}")
    print(f"  • Avg Job Time:  {format_duration(total_elapsed / len(review_ids))}")
    print(f"  • Wrote summary: {summary_file_path}")
    print("================================================")

if __name__ == "__main__":
    main()
