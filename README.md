# Academic Management System (AMS) - Backend API

A robust, high-performance Go REST API powering the Academic Management System. Built with Go (`net/http`), JWT authentication, bcrypt password hashing, role-based access control (Student vs Faculty), PostgreSQL data modeling, and automated email notifications for grade uploads.

## Key Features

- **Multi-Role Authentication**: JWT-based auth with bcrypt password hashing for Student, Faculty, and Admin roles.
- **Student Portal API**: Profile details, complete academic course registration history across all semesters, active semester course registration, and real-time grade transcripts.
- **Faculty Portal API**: Profile details, taught courses (current + history), enrolled student rosters, individual student grade entry, and bulk CSV grade upload with per-row validation reporting.
- **Automated Email Notifications**: Server-side grade notification system dispatching emails to students upon grade publish/update, with support for live SMTP or fallback stdout audit logging.
- **Dual Database Engine**: Automatically connects to PostgreSQL (`DATABASE_URL`) with schema auto-migration & seeding, with fallback to thread-safe in-memory database mode for zero-dependency evaluation.

---

## Environment Variables

| Variable Name | Default Value | Description |
|---|---|---|
| `PORT` | `8080` | Port for HTTP REST API server |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/ams` | PostgreSQL connection URL |
| `JWT_SECRET` | `ams-secret-key-2026-production-grade` | Secret key used to sign JWT tokens |
| `SMTP_HOST` | *(Optional)* | Outbound SMTP server hostname (e.g. `smtp.gmail.com`) |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USERNAME` | *(Optional)* | SMTP username |
| `SMTP_PASSWORD` | *(Optional)* | SMTP password |
| `SMTP_FROM` | `ams-noreply@university.edu` | From email address |

---

## Pre-loaded Test Accounts

The backend automatically seeds initial test accounts with password `password123`:

| Role | Username / ID | Email | Password |
|---|---|---|---|
| **Student** | `STU101` | `alex@university.edu` | `password123` |
| **Student** | `STU102` | `sarah@university.edu` | `password123` |
| **Faculty** | `FAC201` | `robert@university.edu` | `password123` |
| **Faculty** | `FAC202` | `emily@university.edu` | `password123` |
| **Admin** | `admin` | `admin@university.edu` | `password123` |

---

## Setup & Running Locally

### Prerequisites
- Go 1.22+ installed
- PostgreSQL installed (optional; in-memory database will run if PostgreSQL is absent)

### Steps

1. **Clone & Navigate**:
   ```bash
   cd ams-backend
   ```

2. **Install Dependencies**:
   ```bash
   go mod download
   ```

3. **Run Backend Server**:
   ```bash
   go run main.go
   ```
   The API will start at `http://localhost:8080`.

4. **Verify Health**:
   ```bash
   curl http://localhost:8080/api/health
   ```

---

## Deployment Instructions (Render)

1. Sign in to [Render](https://render.com).
2. Click **New +** -> **Web Service**.
3. Connect your GitHub repository `ams-backend`.
4. Configure service settings:
   - **Environment**: `Go`
   - **Build Command**: `go build -o server main.go`
   - **Start Command**: `./server`
5. Add Environment Variables under **Environment**:
   - `PORT` = `10000` (or leave default assigned by Render)
   - `JWT_SECRET` = `<your-random-secret>`
   - `DATABASE_URL` = `<your-postgresql-url>` (e.g. Neon, Supabase, or Render Postgres)
6. Click **Create Web Service**.

---

## API Endpoints Overview

### Public & Authentication
- `GET  /api/health` - API server status & active database mode
- `GET  /api/db-health` - PostgreSQL connectivity status
- `POST /api/auth/login` - Authenticate user & return JWT token + user profile
- `GET  /api/auth/me` - Get current authenticated user details (Protected)
- `GET  /api/email-logs` - View dispatched grade notification audit logs

### Student Endpoints (Header: `Authorization: Bearer <token>`)
- `GET  /api/student/profile` - View student personal details
- `GET  /api/student/courses` - Complete academic history across all semesters
- `GET  /api/student/available-courses` - Active semester courses open for registration
- `POST /api/student/register` - Register for a course offering in active semester

### Faculty Endpoints (Header: `Authorization: Bearer <token>`)
- `GET  /api/faculty/profile` - View faculty personal details
- `GET  /api/faculty/courses` - Courses currently taught & historical course history
- `GET  /api/faculty/enrolled-students?course_offering_id=X` - Student roster for a course
- `POST /api/faculty/grade` - Upload/update grade for an individual student (Verifies faculty authorization)
- `POST /api/faculty/bulk-grades` - Bulk CSV / JSON upload of course grades with validation failure reporting
