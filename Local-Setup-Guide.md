# Livereview Local Setup

## 1. Set Up a Local PostgreSQL Database

Create a PostgreSQL database for local development.

### Create the Database

```sql
CREATE DATABASE livereview;
```

### Define the Database URL

You will need a `DATABASE_URL` for connecting the application to your local PostgreSQL.

Example:

```env
DATABASE_URL=postgres://postgres:password@localhost:5432/livereview?sslmode=disable
```

- `postgres` → database username
- `password` → database password
- `localhost:5432` → local PostgreSQL host and port
- `livereview` → database name
- `sslmode=disable` → disables SSL for local development

---

## 3. Install GitHub CLI (`gh`)

The setup process requires the GitHub CLI for downloading secrets.

Install it from:

https://cli.github.com/

Verify installation:

```bash
gh --version
```

You may also need to authenticate:

```bash
gh auth login
```

---

## 4. Download Environment Secrets

Run:

```bash
make download-secrets
```

This command downloads the required secrets and generates the `.env` file.

---

## 5. Update the Database URL in `.env`

Open the generated `.env` file.

Find the `DATABASE_URL` entry and replace it with your local PostgreSQL connection string.

Example:

```env
DATABASE_URL=postgres://postgres:password@localhost:5432/livereview?sslmode=disable
```

Make sure this points to the database you created in **Step 2**.

---

## 6. Install River

Run:

```bash
make river-install
```

This installs the required River dependencies.

---

## 7. Run River Migrations

Run:

```bash
make river-migrate
```

This sets up River-related database tables.

---

## 8. Install `dbmate`

Install it from:

https://github.com/amacneil/dbmate

Verify installation:

```bash
dbmate --version
```

---

## 9. Run Database Migrations

Apply all database migrations:

```bash
dbmate up
```

This will create the required database schema.

---

## 10. Build the Application

```bash
make build-with-ui
```

---

## 11. Install `typed` Tool

https://github.com/d1vbyz3r0/typed

```bash
go install github.com/d1vbyz3r0/typed/cmd/typed@latest
go get github.com/d1vbyz3r0/typed@latest
```

---

## 11. Run the Backend

From the project root directory:

```bash
make run
```

This starts the LiveReview backend server.

---

## 12. Run the UI

Open a new terminal:

```bash
cd ui
make run
```

This starts the frontend UI locally.

---

## 13. Setup Niceurl

Run either of these:

```bash
make niceurl
```

```bash
make niceurl2
```

```bash
make niceurl3
```

This exposes your local environment through:

- `manual-talent.apps.hexmos.com`
- `manual-talent2.apps.hexmos.com`
- `manual-talent3.apps.hexmos.com`