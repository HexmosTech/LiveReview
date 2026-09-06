# Database Storage Optimization and Diff Storage Offloading Architecture

## Problem

PostgreSQL stores review records in the reviews table. 
The metadata column of the reviews table previously contained a JSON field named preloaded_changes. 
The preloaded_changes field stored raw Git code diff strings. 
The preloaded_changes field used 791.30 megabytes in PostgreSQL, which accounted for 90.4 percent of the metadata column storage. 
Large diff strings in preloaded_changes created TOAST table bloat in PostgreSQL and added 149.85 megabytes to backup files. 
Storing raw source code diffs in PostgreSQL also created privacy concerns.

Additionally, completed background job records in the river_job table accumulated over time, consuming 263.5 megabytes in PostgreSQL and adding 79.09 megabytes to database backup files due to a default 365-day retention configuration.

```mermaid
flowchart TD
    A["Review Creation & Job Queue Execution"] --> B["Store raw diff in metadata JSONB"]
    A --> C["Retain River Job records for 365 days"]
    B --> D["PostgreSQL TOAST Table Bloat (791 MB)"]
    C --> E["River Job Table Bloat (263 MB)"]
    D --> F["Large Backup Files (314 MB)"]
    E --> F
```

## Solution

The review system moves code diffs out of PostgreSQL TOAST storage into external Blob Storage and configures River background job retention limits. 
The system stores code diffs under the object key path org/<org_id>/review/<review_id>/artifacts/preloaded_changes.json. 
The solution contains five components.

1. Review Worker Offloading
2. API Handler Fallback Strategy
3. Multi-Phase Database Migration Script
4. River Job Queue Retention Strategy
5. Security and Scoping

### Review Worker Offloading

The worker process receives a diff review job. 
The worker processes the diff payload into a list of file changes. 
The worker serializes the code diff list to JSON bytes. 
The worker calls the storage helper to save the JSON payload. 
The storage helper writes the object to Blob Storage under the key path org/<org_id>/review/<review_id>/artifacts/preloaded_changes.json. 
The artifact type name is preloaded_changes. 
The worker removes the preloaded_changes key from the metadata map before saving the review record to PostgreSQL. 
If the Blob Storage write fails, the worker preserves preloaded_changes inside the metadata map to prevent data loss.

```mermaid
flowchart TD
    A["Diff Review Job Queue"] --> B["DiffReviewWorker Process"]
    B --> C["Serialize CodeDiff to JSON"]
    C --> D{"Write to Blob Storage?"}
    D -- "Success" --> E["Save to org/org_id/review/review_id/artifacts/preloaded_changes.json"]
    E --> F["Remove preloaded_changes from metadata"]
    D -- "Failure" --> G["Preserve preloaded_changes in Postgres metadata"]
    F --> H["Save Review metadata to PostgreSQL"]
    G --> H
```

### API Handler Fallback Strategy

The API server handles requests for review details for both Web UI and CLI. 
When a client requests a review diff, the server reads the diff artifact from Blob Storage. 
The server uses the key path org/<org_id>/review/<review_id>/artifacts/preloaded_changes.json. 
If the artifact exists in Blob Storage, the server returns the diff data to the client. 
If the artifact does not exist in Blob Storage, the server falls back to read the preloaded_changes field from the metadata column in PostgreSQL. 
This dual-reading fallback strategy ensures that historical reviews and CLI queries remain readable without breaking changes.

```mermaid
flowchart TD
    A["Client GET /api/v1/diff-review/:id"] --> B["Resolve org_id from JWT Context"]
    B --> C["Query Review by ID and org_id"]
    C --> D{"Review exists for org_id?"}
    D -- "No" --> E["Return HTTP 404 Not Found"]
    D -- "Yes" --> F{"Read preloaded_changes.json from Blob Storage"}
    F -- "Found in Blob Storage" --> G["Return CodeDiff payload to Client"]
    F -- "Not Found in Blob Storage" --> H["Fallback: Read preloaded_changes from Postgres metadata"]
    H --> G
```

### Safe Multi-Phase Database Migration Procedure

You run the multi-phase database migration using the script in scripts/migrate_diffs_to_blobstore.go. 
Follow this seven-step procedure to migrate data safely without data loss.

#### Step 1: Create a Local PostgreSQL Backup Dump

Before running the migration, export a local PostgreSQL backup dump of the reviews table.

```bash
pg_dump -h localhost -U livereview -d livereview -t reviews -Fc -f backup_reviews_before_migration.dump
```

#### Step 2: Run Migration Copy Phase

Run the migration script in copy mode. 
Copy mode uploads diff payloads to Blob Storage under org/<org_id>/review/<review_id>/artifacts/preloaded_changes.json. 
Copy mode does not delete preloaded_changes from PostgreSQL.

```bash
go run scripts/migrate_diffs_to_blobstore.go -mode=copy -db="postgres://user:password@localhost:5432/dbname?sslmode=disable" -workers=50
```

#### Step 3: Verify Blob Storage Objects

Verify that the preloaded_changes artifacts exist in Blob Storage. 
Confirm that the object size and object count match the database review count.

#### Step 4: Check for Newly Triggered Reviews

Query PostgreSQL to verify whether new reviews ran while the copy phase executed. 
Confirm that newly created reviews wrote their preloaded_changes artifacts directly to Blob Storage.

#### Step 5: User Verification and Approval

Submit the migration verification report to the user. 
Wait for explicit approval from the user before proceeding to the cleanup step.

#### Step 6: Run Migration Cleanup Phase

After receiving user approval, run the migration script in clean mode. 
Clean mode verifies that the blob object exists before removing preloaded_changes from PostgreSQL metadata.

```bash
go run scripts/migrate_diffs_to_blobstore.go -mode=clean -db="postgres://user:password@localhost:5432/dbname?sslmode=disable" -workers=50
```

#### Step 7: Reclaim PostgreSQL Disk Space

Execute the database vacuum command on the reviews table to reclaim PostgreSQL TOAST disk space.

```sql
VACUUM FULL reviews;
```

```mermaid
flowchart TD
    A["Step 1: Local pg_dump Backup"] --> B["Step 2: Run Script -mode=copy"]
    B --> C["Step 3: Verify Blob Storage Artifacts"]
    C --> D["Step 4: Check New Review Triggers"]
    D --> E["Step 5: User Verification & Approval"]
    E --> F["Step 6: Run Script -mode=clean"]
    F --> G["Step 7: Run VACUUM FULL reviews"]
```

### River Job Queue Retention Strategy

The background job queue system configures River job auto-cleaning to purge historical job records from PostgreSQL after 30 days. 
The queue initialization configures `CompletedJobRetentionPeriod`, `CancelledJobRetentionPeriod`, and `DiscardedJobRetentionPeriod` to 30 days (`30 * 24 * time.Hour`) in `internal/jobqueue/jobqueue.go`. 
This policy purges completed, cancelled, and discarded job records automatically, keeping `river_job` table storage below 1.0 megabyte without manual database maintenance.

### Storage Reduction Results

Running the migration, job retention configuration, and database cleanup procedure produces the following storage reductions.

1. The `reviews` table size in PostgreSQL drops from 880 megabytes to 1.8 megabytes (99.8 percent reduction).
2. The `reviews` table backup dump file drops from 165.72 megabytes to 15.87 megabytes (90.4 percent reduction).
3. The `river_job` table size in PostgreSQL drops from 263 megabytes to under 1.0 megabyte (99.6 percent reduction).
4. The total PostgreSQL database backup dump file size (`livereview.dump`) drops from 327.48 megabytes to under 98.50 megabytes (70.0 percent reduction).

### Security and Scoping

The system enforces organization scoping on all storage keys. 
The storage key includes the organization identifier org_id. 
The API server resolves org_id from the authenticated request context. 
If a user requests a review from another organization, the API server rejects the request with a 404 response.
