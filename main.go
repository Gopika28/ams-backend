package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Package globals & configuration

var (
	db           *pgxpool.Pool
	useMemoryDB  bool
	memMutex     sync.RWMutex
	jwtSecret    = []byte(getEnv("JWT_SECRET", "ams-secret-key-2026-production-grade"))
	smtpHost     = os.Getenv("SMTP_HOST")
	smtpPort     = getEnv("SMTP_PORT", "587")
	smtpUsername = os.Getenv("SMTP_USERNAME")
	smtpPassword = os.Getenv("SMTP_PASSWORD")
	smtpFrom     = getEnv("SMTP_FROM", "ams-noreply@university.edu")
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// Domain models

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // student, faculty, admin
	RefID        int       `json:"ref_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type Student struct {
	ID         int       `json:"id"`
	StudentID  string    `json:"student_id"` // Roll Number
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	Department string    `json:"department"`
	Program    string    `json:"program"`
	Year       int       `json:"year"`
	Section    string    `json:"section"`
	CreatedAt  time.Time `json:"created_at"`
}

type Faculty struct {
	ID          int       `json:"id"`
	FacultyID   string    `json:"faculty_id"` // Employee ID
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Department  string    `json:"department"`
	Designation string    `json:"designation"`
	CreatedAt   time.Time `json:"created_at"`
}

type Semester struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Code             string    `json:"code"`
	IsActive         bool      `json:"is_active"`
	RegistrationOpen bool      `json:"registration_open"`
	CreatedAt        time.Time `json:"created_at"`
}

type Course struct {
	ID          int       `json:"id"`
	CourseID    string    `json:"course_id"`
	CourseName  string    `json:"course_name"`
	Department  string    `json:"department"`
	Credits     int       `json:"credits"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type CourseOffering struct {
	ID          int       `json:"id"`
	CourseID    int       `json:"course_id"`
	SemesterID  int       `json:"semester_id"`
	FacultyID   *int      `json:"faculty_id"`
	MaxCapacity int       `json:"max_capacity"`
	CreatedAt   time.Time `json:"created_at"`

	// Joined fields
	CourseCode   string `json:"course_code,omitempty"`
	CourseName   string `json:"course_name,omitempty"`
	Credits      int    `json:"credits,omitempty"`
	Department   string `json:"department,omitempty"`
	SemesterName string `json:"semester_name,omitempty"`
	FacultyName  string `json:"faculty_name,omitempty"`
	FacultyEmail string `json:"faculty_email,omitempty"`
	IsActive     bool   `json:"is_active,omitempty"`
}

type CourseRegistration struct {
	ID               int       `json:"id"`
	StudentID        int       `json:"student_id"`
	CourseOfferingID int       `json:"course_offering_id"`
	Status           string    `json:"status"` // registered, completed, dropped
	RegistrationDate time.Time `json:"registration_date"`

	// Joined fields
	CourseCode   string  `json:"course_code,omitempty"`
	CourseName   string  `json:"course_name,omitempty"`
	Credits      int     `json:"credits,omitempty"`
	SemesterName string  `json:"semester_name,omitempty"`
	FacultyName  string  `json:"faculty_name,omitempty"`
	Grade        string  `json:"grade,omitempty"`
	Marks        float64 `json:"marks,omitempty"`
}

type Result struct {
	ID               int       `json:"id"`
	StudentID        int       `json:"student_id"`
	CourseOfferingID int       `json:"course_offering_id"`
	Marks            float64   `json:"marks"`
	Grade            string    `json:"grade"`
	Remarks          string    `json:"remarks"`
	SemesterName     string    `json:"semester_name"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Joined fields
	StudentRollNo string `json:"student_roll_no,omitempty"`
	StudentName   string `json:"student_name,omitempty"`
	StudentEmail  string `json:"student_email,omitempty"`
	CourseCode    string `json:"course_code,omitempty"`
	CourseName    string `json:"course_name,omitempty"`
}

type EmailLog struct {
	ID             int       `json:"id"`
	RecipientEmail string    `json:"recipient_email"`
	StudentName    string    `json:"student_name"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	Status         string    `json:"status"`
	SentAt         time.Time `json:"sent_at"`
}

type JWTClaims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	RefID    int    `json:"ref_id"`
	jwt.RegisteredClaims
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type GradeUploadRequest struct {
	CourseOfferingID int     `json:"course_offering_id"`
	StudentRollNo    string  `json:"student_roll_no"` // OR student_id int
	StudentID        int     `json:"student_id"`
	Marks            float64 `json:"marks"`
	Grade            string  `json:"grade"`
	Remarks          string  `json:"remarks"`
}

type BulkGradeRow struct {
	StudentRollNo string  `json:"student_roll_no"`
	Marks         float64 `json:"marks"`
	Grade         string  `json:"grade"`
	Remarks       string  `json:"remarks"`
}

type BulkGradeRequest struct {
	CourseOfferingID int            `json:"course_offering_id"`
	Grades           []BulkGradeRow `json:"grades"`
}

type BulkFailedRow struct {
	Row           int    `json:"row"`
	StudentRollNo string `json:"student_roll_no"`
	Reason        string `json:"reason"`
}

type BulkGradeResponse struct {
	SuccessCount int             `json:"success_count"`
	FailedCount  int             `json:"failed_count"`
	FailedRows   []BulkFailedRow `json:"failed_rows"`
	Message      string          `json:"message"`
}

// In-memory data store for local execution

var (
	userNextID         = 10
	studentNextID      = 10
	facultyNextID      = 10
	semesterNextID     = 10
	courseNextID       = 10
	offeringNextID     = 10
	registrationNextID = 10
	resultNextID       = 10
	emailLogNextID     = 10

	memUsers         []User
	memStudents      []Student
	memFaculty       []Faculty
	memSemesters     []Semester
	memCourses       []Course
	memOfferings     []CourseOffering
	memRegistrations []CourseRegistration
	memResults       []Result
	memEmailLogs     []EmailLog
)

func initInMemoryStore() {
	memMutex.Lock()
	defer memMutex.Unlock()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	passHash := string(hash)

	memStudents = []Student{
		{ID: 1, StudentID: "STU101", Name: "Priya Sharma", Email: "priya@university.edu", Phone: "555-0101", Department: "Computer Science", Program: "B.Tech Computer Science", Year: 2, Section: "A", CreatedAt: time.Now()},
		{ID: 2, StudentID: "STU102", Name: "Meenu Patel", Email: "meenu@university.edu", Phone: "555-0102", Department: "Electrical Engineering", Program: "B.Tech Electrical Engineering", Year: 3, Section: "B", CreatedAt: time.Now()},
		{ID: 3, StudentID: "STU103", Name: "Ananya Reddy", Email: "ananya@university.edu", Phone: "555-0103", Department: "Computer Science", Program: "B.Tech Computer Science", Year: 1, Section: "A", CreatedAt: time.Now()},
		{ID: 4, StudentID: "STU104", Name: "Karthik Kumar", Email: "karthik@university.edu", Phone: "555-0104", Department: "Information Technology", Program: "B.Tech IT", Year: 4, Section: "C", CreatedAt: time.Now()},
		{ID: 5, StudentID: "STU105", Name: "Rahul Verma", Email: "rahul@university.edu", Phone: "555-0105", Department: "Computer Science", Program: "B.Tech AI & ML", Year: 2, Section: "B", CreatedAt: time.Now()},
	}

	memFaculty = []Faculty{
		{ID: 1, FacultyID: "FAC201", Name: "Dr. K. Seshadri", Email: "seshadri@university.edu", Phone: "555-0201", Department: "Computer Science", Designation: "Professor", CreatedAt: time.Now()},
		{ID: 2, FacultyID: "FAC202", Name: "Dr. Meenakshi", Email: "meenakshi@university.edu", Phone: "555-0202", Department: "Electrical Engineering", Designation: "Associate Professor", CreatedAt: time.Now()},
		{ID: 3, FacultyID: "FAC203", Name: "Dr. N. Ramaswamy", Email: "ramaswamy@university.edu", Phone: "555-0203", Department: "Mechanical Engineering", Designation: "Professor", CreatedAt: time.Now()},
		{ID: 4, FacultyID: "FAC204", Name: "Dr. Radhakrishnan", Email: "radhakrishnan@university.edu", Phone: "555-0204", Department: "Information Technology", Designation: "Associate Professor", CreatedAt: time.Now()},
		{ID: 5, FacultyID: "FAC205", Name: "Dr. C. V. Raman", Email: "raman@university.edu", Phone: "555-0205", Department: "Computer Science", Designation: "Assistant Professor", CreatedAt: time.Now()},
	}

	memUsers = []User{
		{ID: 1, Username: "STU101", Email: "priya@university.edu", PasswordHash: passHash, Role: "student", RefID: 1, CreatedAt: time.Now()},
		{ID: 2, Username: "STU102", Email: "meenu@university.edu", PasswordHash: passHash, Role: "student", RefID: 2, CreatedAt: time.Now()},
		{ID: 3, Username: "STU103", Email: "ananya@university.edu", PasswordHash: passHash, Role: "student", RefID: 3, CreatedAt: time.Now()},
		{ID: 4, Username: "STU104", Email: "karthik@university.edu", PasswordHash: passHash, Role: "student", RefID: 4, CreatedAt: time.Now()},
		{ID: 5, Username: "STU105", Email: "rahul@university.edu", PasswordHash: passHash, Role: "student", RefID: 5, CreatedAt: time.Now()},
		{ID: 6, Username: "FAC201", Email: "seshadri@university.edu", PasswordHash: passHash, Role: "faculty", RefID: 1, CreatedAt: time.Now()},
		{ID: 7, Username: "FAC202", Email: "meenakshi@university.edu", PasswordHash: passHash, Role: "faculty", RefID: 2, CreatedAt: time.Now()},
		{ID: 8, Username: "FAC203", Email: "ramaswamy@university.edu", PasswordHash: passHash, Role: "faculty", RefID: 3, CreatedAt: time.Now()},
		{ID: 9, Username: "FAC204", Email: "radhakrishnan@university.edu", PasswordHash: passHash, Role: "faculty", RefID: 4, CreatedAt: time.Now()},
		{ID: 10, Username: "FAC205", Email: "raman@university.edu", PasswordHash: passHash, Role: "faculty", RefID: 5, CreatedAt: time.Now()},
		{ID: 11, Username: "admin", Email: "admin@university.edu", PasswordHash: passHash, Role: "admin", RefID: 0, CreatedAt: time.Now()},
	}

	memSemesters = []Semester{
		{ID: 1, Name: "Semester 1", Code: "SEM1", IsActive: false, RegistrationOpen: false, CreatedAt: time.Now()},
		{ID: 2, Name: "Semester 2", Code: "SEM2", IsActive: true, RegistrationOpen: true, CreatedAt: time.Now()},
	}

	memCourses = []Course{
		{ID: 1, CourseID: "CS101", CourseName: "Data Structures & Algorithms", Department: "Computer Science", Credits: 4, Description: "Fundamental concepts of data structures, complexity analysis, and algorithms.", CreatedAt: time.Now()},
		{ID: 2, CourseID: "EE201", CourseName: "Circuit Analysis", Department: "Electrical Engineering", Credits: 3, Description: "Principles of linear circuit analysis, Kirchhoff laws, and network theorems.", CreatedAt: time.Now()},
		{ID: 3, CourseID: "CS202", CourseName: "Database Management Systems", Department: "Computer Science", Credits: 4, Description: "Relational databases, SQL query optimization, indexing, and transactions.", CreatedAt: time.Now()},
		{ID: 4, CourseID: "EE301", CourseName: "Digital Signal Processing", Department: "Electrical Engineering", Credits: 4, Description: "Signals and systems, Fourier transform, Z-transform, and digital filter design.", CreatedAt: time.Now()},
		{ID: 5, CourseID: "ME101", CourseName: "Engineering Mechanics", Department: "Mechanical Engineering", Credits: 3, Description: "Statics, dynamics, stress-strain analysis, and force systems.", CreatedAt: time.Now()},
		{ID: 6, CourseID: "IT201", CourseName: "Web Development & Cloud", Department: "Information Technology", Credits: 4, Description: "Full-stack development, modern APIs, microservices, and cloud deployments.", CreatedAt: time.Now()},
		{ID: 7, CourseID: "AI301", CourseName: "Generative AI & Large Language Models", Department: "Computer Science", Credits: 4, Description: "Architectures of Transformer models, Prompt Engineering, Retrieval-Augmented Generation (RAG), Fine-Tuning LLMs, and AI Safety.", CreatedAt: time.Now()},
		{ID: 8, CourseID: "CYB302", CourseName: "Cyber Crime Investigation & Digital Forensics", Department: "Information Technology", Credits: 4, Description: "Techniques for digital evidence extraction, chain of custody, memory analysis, disk forensics, malware triage, and cyber threat intelligence.", CreatedAt: time.Now()},
		{ID: 9, CourseID: "CYB401", CourseName: "Ethical Hacking & Penetration Testing", Department: "Information Technology", Credits: 4, Description: "Vulnerability assessment, network penetration testing, web application security auditing, exploit development, and defense countermeasures.", CreatedAt: time.Now()},
		{ID: 10, CourseID: "AI402", CourseName: "Applied Deep Learning & Computer Vision", Department: "Computer Science", Credits: 4, Description: "Convolutional Neural Networks (CNNs), Object Detection, Image Segmentation, GANs, and real-time video analytics.", CreatedAt: time.Now()},
		{ID: 11, CourseID: "IT305", CourseName: "Cloud Cyber Threat Defense & Incident Response", Department: "Information Technology", Credits: 3, Description: "Securing AWS/Azure cloud infrastructure, Zero Trust architecture, SIEM log monitoring, threat hunting, and rapid incident response.", CreatedAt: time.Now()},
	}

	fac1ID := 1
	fac2ID := 2
	fac3ID := 3
	fac4ID := 4
	fac5ID := 5

	memOfferings = []CourseOffering{
		{ID: 1, CourseID: 1, SemesterID: 2, FacultyID: &fac1ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 2, CourseID: 2, SemesterID: 2, FacultyID: &fac2ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 3, CourseID: 3, SemesterID: 1, FacultyID: &fac1ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 4, CourseID: 4, SemesterID: 1, FacultyID: &fac2ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 5, CourseID: 5, SemesterID: 2, FacultyID: &fac3ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 6, CourseID: 6, SemesterID: 2, FacultyID: &fac4ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 7, CourseID: 7, SemesterID: 2, FacultyID: &fac1ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 8, CourseID: 8, SemesterID: 2, FacultyID: &fac4ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 9, CourseID: 9, SemesterID: 2, FacultyID: &fac4ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 10, CourseID: 10, SemesterID: 2, FacultyID: &fac5ID, MaxCapacity: 60, CreatedAt: time.Now()},
		{ID: 11, CourseID: 11, SemesterID: 2, FacultyID: &fac4ID, MaxCapacity: 60, CreatedAt: time.Now()},
	}

	memRegistrations = []CourseRegistration{
		{ID: 1, StudentID: 1, CourseOfferingID: 1, Status: "registered", RegistrationDate: time.Now()},
		{ID: 2, StudentID: 1, CourseOfferingID: 3, Status: "completed", RegistrationDate: time.Now().Add(-120 * 24 * time.Hour)},
		{ID: 3, StudentID: 2, CourseOfferingID: 2, Status: "registered", RegistrationDate: time.Now()},
		{ID: 4, StudentID: 2, CourseOfferingID: 4, Status: "completed", RegistrationDate: time.Now().Add(-120 * 24 * time.Hour)},
		{ID: 5, StudentID: 3, CourseOfferingID: 5, Status: "registered", RegistrationDate: time.Now()},
		{ID: 6, StudentID: 4, CourseOfferingID: 1, Status: "registered", RegistrationDate: time.Now()},
		{ID: 7, StudentID: 5, CourseOfferingID: 6, Status: "registered", RegistrationDate: time.Now()},
	}

	memResults = []Result{
		{ID: 1, StudentID: 1, CourseOfferingID: 3, Marks: 92.5, Grade: "A+", Remarks: "Outstanding performance in algorithms", SemesterName: "Semester 1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: 2, StudentID: 2, CourseOfferingID: 4, Marks: 84.0, Grade: "A", Remarks: "Excellent lab coursework and exam result", SemesterName: "Semester 1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
}

// PostgreSQL initialization

func initTables(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(100) UNIQUE NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role VARCHAR(20) NOT NULL CHECK (role IN ('student', 'faculty', 'admin')),
			ref_id INT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS students (
			id SERIAL PRIMARY KEY,
			student_id VARCHAR(50) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			phone VARCHAR(20),
			department VARCHAR(100) NOT NULL,
			program VARCHAR(100) DEFAULT 'B.Tech Computer Science',
			year INT DEFAULT 1,
			section VARCHAR(10) DEFAULT 'A',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS faculty (
			id SERIAL PRIMARY KEY,
			faculty_id VARCHAR(50) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			phone VARCHAR(20),
			department VARCHAR(100) NOT NULL,
			designation VARCHAR(100) DEFAULT 'Professor',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS semesters (
			id SERIAL PRIMARY KEY,
			name VARCHAR(50) NOT NULL,
			code VARCHAR(20) UNIQUE NOT NULL,
			is_active BOOLEAN DEFAULT false,
			registration_open BOOLEAN DEFAULT false,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS courses (
			id SERIAL PRIMARY KEY,
			course_id VARCHAR(50) UNIQUE NOT NULL,
			course_name VARCHAR(100) NOT NULL,
			department VARCHAR(100) NOT NULL,
			credits INT NOT NULL DEFAULT 4,
			description TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS course_offerings (
			id SERIAL PRIMARY KEY,
			course_id INT REFERENCES courses(id) ON DELETE CASCADE,
			semester_id INT REFERENCES semesters(id) ON DELETE CASCADE,
			faculty_id INT REFERENCES faculty(id) ON DELETE SET NULL,
			max_capacity INT DEFAULT 60,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT unique_course_semester UNIQUE(course_id, semester_id)
		);`,
		`CREATE TABLE IF NOT EXISTS course_registrations (
			id SERIAL PRIMARY KEY,
			student_id INT REFERENCES students(id) ON DELETE CASCADE,
			course_offering_id INT REFERENCES course_offerings(id) ON DELETE CASCADE,
			status VARCHAR(20) DEFAULT 'registered',
			registration_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT unique_student_offering UNIQUE(student_id, course_offering_id)
		);`,
		`CREATE TABLE IF NOT EXISTS results (
			id SERIAL PRIMARY KEY,
			student_id INT REFERENCES students(id) ON DELETE CASCADE,
			course_offering_id INT REFERENCES course_offerings(id) ON DELETE CASCADE,
			marks NUMERIC(5,2) NOT NULL,
			grade VARCHAR(10) NOT NULL,
			remarks TEXT,
			semester_name VARCHAR(50),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT unique_student_offering_grade UNIQUE(student_id, course_offering_id)
		);`,
		`CREATE TABLE IF NOT EXISTS email_logs (
			id SERIAL PRIMARY KEY,
			recipient_email VARCHAR(100) NOT NULL,
			student_name VARCHAR(100) NOT NULL,
			subject VARCHAR(200) NOT NULL,
			body TEXT NOT NULL,
			status VARCHAR(20) DEFAULT 'sent',
			sent_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, query := range queries {
		_, err := db.Exec(ctx, query)
		if err != nil {
			return err
		}
	}

	seedDatabaseIfEmpty(ctx)
	return nil
}

func seedDatabaseIfEmpty(parentCtx context.Context) {
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer seedCancel()

	log.Println("Unconditionally running database seed & sync in PostgreSQL...")
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	passHash := string(hash)

	// 1. Ensure Students Exist & Sync Names
	db.Exec(seedCtx, `INSERT INTO students (id, student_id, name, email, phone, department, program, year, section) VALUES
		(1, 'STU101', 'Priya Sharma', 'priya@university.edu', '555-0101', 'Computer Science', 'B.Tech Computer Science', 2, 'A'),
		(2, 'STU102', 'Meenu Patel', 'meenu@university.edu', '555-0102', 'Electrical Engineering', 'B.Tech Electrical Engineering', 3, 'B'),
		(3, 'STU103', 'Ananya Reddy', 'ananya@university.edu', '555-0103', 'Computer Science', 'B.Tech Computer Science', 1, 'A'),
		(4, 'STU104', 'Karthik Kumar', 'karthik@university.edu', '555-0104', 'Information Technology', 'B.Tech IT', 4, 'C'),
		(5, 'STU105', 'Rahul Verma', 'rahul@university.edu', '555-0105', 'Computer Science', 'B.Tech AI & ML', 2, 'B')
		ON CONFLICT DO NOTHING;`)

	db.Exec(seedCtx, `UPDATE students SET name = 'Priya Sharma', email = 'priya@university.edu' WHERE id = 1 OR student_id = 'STU101';`)
	db.Exec(seedCtx, `UPDATE students SET name = 'Meenu Patel', email = 'meenu@university.edu' WHERE id = 2 OR student_id = 'STU102';`)
	db.Exec(seedCtx, `UPDATE students SET name = 'Ananya Reddy', email = 'ananya@university.edu' WHERE id = 3 OR student_id = 'STU103';`)
	db.Exec(seedCtx, `UPDATE students SET name = 'Karthik Kumar', email = 'karthik@university.edu' WHERE id = 4 OR student_id = 'STU104';`)
	db.Exec(seedCtx, `UPDATE students SET name = 'Rahul Verma', email = 'rahul@university.edu' WHERE id = 5 OR student_id = 'STU105';`)

	// 2. Ensure Faculty Exist & Sync Names
	db.Exec(seedCtx, `INSERT INTO faculty (id, faculty_id, name, email, phone, department, designation) VALUES
		(1, 'FAC201', 'Dr. K. Seshadri', 'seshadri@university.edu', '555-0201', 'Computer Science', 'Professor'),
		(2, 'FAC202', 'Dr. Meenakshi', 'meenakshi@university.edu', '555-0202', 'Electrical Engineering', 'Associate Professor'),
		(3, 'FAC203', 'Dr. N. Ramaswamy', 'ramaswamy@university.edu', '555-0203', 'Mechanical Engineering', 'Professor'),
		(4, 'FAC204', 'Dr. Radhakrishnan', 'radhakrishnan@university.edu', '555-0204', 'Information Technology', 'Associate Professor'),
		(5, 'FAC205', 'Dr. C. V. Raman', 'raman@university.edu', '555-0205', 'Computer Science', 'Assistant Professor')
		ON CONFLICT DO NOTHING;`)

	db.Exec(seedCtx, `UPDATE faculty SET name = 'Dr. K. Seshadri', email = 'seshadri@university.edu' WHERE id = 1 OR faculty_id = 'FAC201';`)
	db.Exec(seedCtx, `UPDATE faculty SET name = 'Dr. Meenakshi', email = 'meenakshi@university.edu' WHERE id = 2 OR faculty_id = 'FAC202';`)
	db.Exec(seedCtx, `UPDATE faculty SET name = 'Dr. N. Ramaswamy', email = 'ramaswamy@university.edu' WHERE id = 3 OR faculty_id = 'FAC203';`)
	db.Exec(seedCtx, `UPDATE faculty SET name = 'Dr. Radhakrishnan', email = 'radhakrishnan@university.edu' WHERE id = 4 OR faculty_id = 'FAC204';`)
	db.Exec(seedCtx, `UPDATE faculty SET name = 'Dr. C. V. Raman', email = 'raman@university.edu' WHERE id = 5 OR faculty_id = 'FAC205';`)

	// 3. Ensure Semesters Exist
	db.Exec(seedCtx, `INSERT INTO semesters (id, name, code, is_active, registration_open) VALUES
		(1, 'Semester 1', 'SEM1', false, false),
		(2, 'Semester 2', 'SEM2', true, true)
		ON CONFLICT DO NOTHING;`)

	// 4. Ensure Courses Exist
	db.Exec(seedCtx, `INSERT INTO courses (id, course_id, course_name, department, credits, description) VALUES
		(1, 'CS101', 'Data Structures & Algorithms', 'Computer Science', 4, 'Fundamental concepts of data structures, complexity analysis, and algorithms.'),
		(2, 'EE201', 'Circuit Analysis', 'Electrical Engineering', 3, 'Principles of linear circuit analysis, Kirchhoff laws, and network theorems.'),
		(3, 'CS202', 'Database Management Systems', 'Computer Science', 4, 'Relational databases, SQL query optimization, indexing, and transactions.'),
		(4, 'EE301', 'Digital Signal Processing', 'Electrical Engineering', 4, 'Signals and systems, Fourier transform, Z-transform, and digital filter design.'),
		(5, 'ME101', 'Engineering Mechanics', 'Mechanical Engineering', 3, 'Statics, dynamics, stress-strain analysis, and force systems.'),
		(6, 'IT201', 'Web Development & Cloud', 'Information Technology', 4, 'Full-stack development, modern APIs, microservices, and cloud deployments.'),
		(7, 'AI301', 'Generative AI & Large Language Models', 'Computer Science', 4, 'Architectures of Transformer models, Prompt Engineering, Retrieval-Augmented Generation (RAG), Fine-Tuning LLMs, and AI Safety.'),
		(8, 'CYB302', 'Cyber Crime Investigation & Digital Forensics', 'Information Technology', 4, 'Techniques for digital evidence extraction, chain of custody, memory analysis, disk forensics, malware triage, and cyber threat intelligence.'),
		(9, 'CYB401', 'Ethical Hacking & Penetration Testing', 'Information Technology', 4, 'Vulnerability assessment, network penetration testing, web application security auditing, exploit development, and defense countermeasures.'),
		(10, 'AI402', 'Applied Deep Learning & Computer Vision', 'Computer Science', 4, 'Convolutional Neural Networks (CNNs), Object Detection, Image Segmentation, GANs, and real-time video analytics.'),
		(11, 'IT305', 'Cloud Cyber Threat Defense & Incident Response', 'Information Technology', 3, 'Securing AWS/Azure cloud infrastructure, Zero Trust architecture, SIEM log monitoring, threat hunting, and rapid incident response.')
		ON CONFLICT DO NOTHING;`)

	// 5. Ensure Course Offerings Exist & Sync Faculty Assignments
	db.Exec(seedCtx, `INSERT INTO course_offerings (id, course_id, semester_id, faculty_id, max_capacity) VALUES
		(1, 1, 2, 1, 60),
		(2, 2, 2, 2, 60),
		(3, 3, 1, 1, 60),
		(4, 4, 1, 2, 60),
		(5, 5, 2, 3, 60),
		(6, 6, 2, 4, 60),
		(7, 7, 2, 1, 60),
		(8, 8, 2, 4, 60),
		(9, 9, 2, 4, 60),
		(10, 10, 2, 5, 60),
		(11, 11, 2, 4, 60)
		ON CONFLICT (id) DO UPDATE SET faculty_id = EXCLUDED.faculty_id, semester_id = EXCLUDED.semester_id;`)

	// 6. Ensure Registrations Exist & Sync Status
	db.Exec(seedCtx, `INSERT INTO course_registrations (id, student_id, course_offering_id, status) VALUES
		(1, 1, 1, 'registered'),
		(2, 1, 7, 'registered'),
		(3, 1, 8, 'registered'),
		(4, 1, 3, 'completed'),
		(5, 2, 2, 'registered'),
		(6, 2, 9, 'registered'),
		(7, 2, 4, 'completed'),
		(8, 3, 1, 'registered'),
		(9, 3, 10, 'registered'),
		(10, 4, 6, 'registered'),
		(11, 4, 11, 'registered'),
		(12, 5, 5, 'registered'),
		(13, 5, 7, 'registered')
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;`)

	// 7. Ensure Results Exist & Sync Grade
	db.Exec(seedCtx, `INSERT INTO results (id, student_id, course_offering_id, marks, grade, remarks, semester_name) VALUES
		(1, 1, 3, 95.0, 'A+', 'Outstanding performance in algorithms', 'Semester 1'),
		(2, 2, 4, 88.0, 'A', 'Excellent lab coursework and exam result', 'Semester 1')
		ON CONFLICT (id) DO UPDATE SET marks = EXCLUDED.marks, grade = EXCLUDED.grade;`)

	// 8. Ensure Users Exist & Sync Emails and Passwords
	db.Exec(seedCtx, `INSERT INTO users (id, username, email, password_hash, role, ref_id) VALUES
		(1, 'STU101', 'priya@university.edu', $1, 'student', 1),
		(2, 'STU102', 'meenu@university.edu', $1, 'student', 2),
		(3, 'STU103', 'ananya@university.edu', $1, 'student', 3),
		(4, 'STU104', 'karthik@university.edu', $1, 'student', 4),
		(5, 'STU105', 'rahul@university.edu', $1, 'student', 5),
		(6, 'FAC201', 'seshadri@university.edu', $1, 'faculty', 1),
		(7, 'FAC202', 'meenakshi@university.edu', $1, 'faculty', 2),
		(8, 'FAC203', 'ramaswamy@university.edu', $1, 'faculty', 3),
		(9, 'FAC204', 'radhakrishnan@university.edu', $1, 'faculty', 4),
		(10, 'FAC205', 'raman@university.edu', $1, 'faculty', 5),
		(11, 'admin', 'admin@university.edu', $1, 'admin', 0)
		ON CONFLICT (username) DO UPDATE SET password_hash = EXCLUDED.password_hash, email = EXCLUDED.email;`, passHash)

	db.Exec(seedCtx, `UPDATE users SET password_hash = $1 WHERE username IN ('STU101','STU102','STU103','STU104','STU105','FAC201','FAC202','FAC203','FAC204','FAC205','admin');`, passHash)
	db.Exec(seedCtx, `UPDATE users SET email = 'priya@university.edu' WHERE id = 1 OR username = 'STU101';`)
	db.Exec(seedCtx, `UPDATE users SET email = 'meenu@university.edu' WHERE id = 2 OR username = 'STU102';`)
	db.Exec(seedCtx, `UPDATE users SET email = 'ananya@university.edu' WHERE id = 3 OR username = 'STU103';`)
	db.Exec(seedCtx, `UPDATE users SET email = 'karthik@university.edu' WHERE id = 4 OR username = 'STU104';`)
	db.Exec(seedCtx, `UPDATE users SET email = 'rahul@university.edu' WHERE id = 5 OR username = 'STU105';`)
}

// Authentication and token utilities

func generateToken(user User) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
		RefID:    user.RefID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

type contextKey string

const userContextKey contextKey = "authenticatedUser"

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"Authorization token required"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error":"Invalid authorization header format"}`, http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims := &JWTClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, `{"error":"Invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func getClaimsFromContext(r *http.Request) (*JWTClaims, bool) {
	claims, ok := r.Context().Value(userContextKey).(*JWTClaims)
	return claims, ok
}

// Email service

func sendGradeNotification(recipientEmail, studentName, courseCode, courseName, grade string, marks float64, semesterName string) {
	subject := fmt.Sprintf("Academic Management System: Grade Published for %s", courseCode)
	body := fmt.Sprintf(`Dear %s,

Your grade for the course %s - %s (%s) has been published/updated.

Academic Details:
- Course: %s - %s
- Semester: %s
- Marks Obtained: %.2f / 100
- Final Grade: %s

This update is now reflected on your AMS Academic Portal transcript.

Best regards,
Office of Academic Affairs
University Portal`, studentName, courseCode, courseName, semesterName, courseCode, courseName, semesterName, marks, grade)

	log.Printf("[EMAIL NOTIFICATION] Triggered for %s (%s) | Course: %s | Grade: %s | Marks: %.2f", studentName, recipientEmail, courseCode, grade, marks)

	// Persist email log
	if useMemoryDB {
		memMutex.Lock()
		logEntry := EmailLog{
			ID:             emailLogNextID,
			RecipientEmail: recipientEmail,
			StudentName:    studentName,
			Subject:        subject,
			Body:           body,
			Status:         "Sent (Simulated)",
			SentAt:         time.Now(),
		}
		emailLogNextID++
		memEmailLogs = append(memEmailLogs, logEntry)
		memMutex.Unlock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		db.Exec(ctx, `
			INSERT INTO email_logs (recipient_email, student_name, subject, body, status)
			VALUES ($1, $2, $3, $4, $5)
		`, recipientEmail, studentName, subject, body, "Sent")
	}

	// Real SMTP send if credentials configured
	if smtpHost != "" {
		go func() {
			addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)
			msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", smtpFrom, recipientEmail, subject, body)
			var auth smtp.Auth
			if smtpUsername != "" {
				auth = smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)
			}
			err := smtp.SendMail(addr, auth, smtpFrom, []string{recipientEmail}, []byte(msg))
			if err != nil {
				log.Printf("[SMTP ERROR] Failed to send email to %s: %v", recipientEmail, err)
			} else {
				log.Printf("[SMTP SUCCESS] Email successfully delivered to %s", recipientEmail)
			}
		}()
	}
}

// Auth HTTP handlers

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)

	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"Username and password are required"}`, http.StatusBadRequest)
		return
	}

	var user User
	var found bool

	if useMemoryDB {
		memMutex.RLock()
		for _, u := range memUsers {
			if strings.EqualFold(u.Username, req.Username) || strings.EqualFold(u.Email, req.Username) {
				user = u
				found = true
				break
			}
		}
		memMutex.RUnlock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := db.QueryRow(ctx, `
			SELECT id, username, email, password_hash, role, ref_id, created_at
			FROM users WHERE LOWER(username) = LOWER($1) OR LOWER(email) = LOWER($1)
		`, req.Username).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.RefID, &user.CreatedAt)

		if err == nil {
			found = true
		} else {
			log.Printf("[LOGIN SERVER ERROR] QueryRow for username=%q failed: %v", req.Username, err)
		}
	}

	if !found {
		log.Printf("[LOGIN SERVER] User not found for username=%q", req.Username)
		http.Error(w, `{"error":"Invalid username/email or password"}`, http.StatusUnauthorized)
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		log.Printf("[LOGIN SERVER] Bcrypt match failed for username=%q (dbHash=%q passLen=%d): %v", user.Username, user.PasswordHash, len(req.Password), err)
		http.Error(w, `{"error":"Invalid username/email or password"}`, http.StatusUnauthorized)
		return
	}
	log.Printf("[LOGIN SERVER] Login SUCCESS for username=%q role=%s", user.Username, user.Role)

	token, err := generateToken(user)
	if err != nil {
		http.Error(w, `{"error":"Failed to generate auth token"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(LoginResponse{
		Token: token,
		User:  user,
	})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, ok := getClaimsFromContext(r)
	if !ok {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"email":    claims.Email,
		"role":     claims.Role,
		"ref_id":   claims.RefID,
	})
}

// Student HTTP handlers

func studentProfileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "student" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Student access required"}`, http.StatusForbidden)
		return
	}

	studentID := claims.RefID
	if qID := r.URL.Query().Get("student_id"); qID != "" && claims.Role == "admin" {
		studentID, _ = strconv.Atoi(qID)
	}

	var s Student
	var found bool

	if useMemoryDB {
		memMutex.RLock()
		for _, st := range memStudents {
			if st.ID == studentID {
				s = st
				found = true
				break
			}
		}
		memMutex.RUnlock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := db.QueryRow(ctx, `
			SELECT id, student_id, name, email, phone, department, program, year, section, created_at
			FROM students WHERE id = $1
		`, studentID).Scan(&s.ID, &s.StudentID, &s.Name, &s.Email, &s.Phone, &s.Department, &s.Program, &s.Year, &s.Section, &s.CreatedAt)
		if err == nil {
			found = true
		}
	}

	if !found {
		http.Error(w, `{"error":"Student profile record not found"}`, http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(s)
}

func studentCoursesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "student" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Student access required"}`, http.StatusForbidden)
		return
	}

	studentID := claims.RefID

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()

		var history []CourseRegistration
		for _, reg := range memRegistrations {
			if reg.StudentID == studentID {
				item := reg
				// Enrich with course & semester details
				for _, off := range memOfferings {
					if off.ID == reg.CourseOfferingID {
						for _, c := range memCourses {
							if c.ID == off.CourseID {
								item.CourseCode = c.CourseID
								item.CourseName = c.CourseName
								item.Credits = c.Credits
							}
						}
						for _, sem := range memSemesters {
							if sem.ID == off.SemesterID {
								item.SemesterName = sem.Name
							}
						}
						if off.FacultyID != nil {
							for _, f := range memFaculty {
								if f.ID == *off.FacultyID {
									item.FacultyName = f.Name
								}
							}
						}
					}
				}
				// Enrich grade
				for _, res := range memResults {
					if res.StudentID == studentID && res.CourseOfferingID == reg.CourseOfferingID {
						item.Grade = res.Grade
						item.Marks = res.Marks
					}
				}
				history = append(history, item)
			}
		}
		json.NewEncoder(w).Encode(history)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT 
			cr.id, cr.student_id, cr.course_offering_id, cr.status, cr.registration_date,
			c.course_id, c.course_name, c.credits, s.name, COALESCE(f.name, 'Unassigned'),
			COALESCE(r.grade, ''), COALESCE(r.marks::float8, 0.0)
		FROM course_registrations cr
		JOIN course_offerings co ON cr.course_offering_id = co.id
		JOIN courses c ON co.course_id = c.id
		JOIN semesters s ON co.semester_id = s.id
		LEFT JOIN faculty f ON co.faculty_id = f.id
		LEFT JOIN results r ON (r.student_id = cr.student_id AND r.course_offering_id = cr.course_offering_id)
		WHERE cr.student_id = $1
		ORDER BY s.id DESC, c.course_id ASC
	`, studentID)

	if err != nil {
		http.Error(w, `{"error":"Failed to fetch registered courses"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []CourseRegistration{}
	for rows.Next() {
		var item CourseRegistration
		err := rows.Scan(&item.ID, &item.StudentID, &item.CourseOfferingID, &item.Status, &item.RegistrationDate,
			&item.CourseCode, &item.CourseName, &item.Credits, &item.SemesterName, &item.FacultyName,
			&item.Grade, &item.Marks)
		if err != nil {
			http.Error(w, `{"error":"Data scan error"}`, http.StatusInternalServerError)
			return
		}
		list = append(list, item)
	}

	json.NewEncoder(w).Encode(list)
}

func studentAvailableCoursesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "student" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Student access required"}`, http.StatusForbidden)
		return
	}

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()

		var activeSemID int
		var activeSemName string
		for _, sem := range memSemesters {
			if sem.IsActive && sem.RegistrationOpen {
				activeSemID = sem.ID
				activeSemName = sem.Name
				break
			}
		}

		var available []CourseOffering
		for _, off := range memOfferings {
			if off.SemesterID == activeSemID {
				item := off
				item.SemesterName = activeSemName
				for _, c := range memCourses {
					if c.ID == off.CourseID {
						item.CourseCode = c.CourseID
						item.CourseName = c.CourseName
						item.Credits = c.Credits
						item.Department = c.Department
					}
				}
				if off.FacultyID != nil {
					for _, f := range memFaculty {
						if f.ID == *off.FacultyID {
							item.FacultyName = f.Name
							item.FacultyEmail = f.Email
						}
					}
				}
				available = append(available, item)
			}
		}
		json.NewEncoder(w).Encode(available)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT 
			co.id, co.course_id, co.semester_id, co.faculty_id, co.max_capacity,
			c.course_id, c.course_name, c.credits, c.department, s.name, COALESCE(f.name, 'Unassigned'), COALESCE(f.email, '')
		FROM course_offerings co
		JOIN courses c ON co.course_id = c.id
		JOIN semesters s ON co.semester_id = s.id
		LEFT JOIN faculty f ON co.faculty_id = f.id
		WHERE s.is_active = true AND s.registration_open = true
	`)
	if err != nil {
		http.Error(w, `{"error":"Failed to fetch available courses"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []CourseOffering{}
	for rows.Next() {
		var item CourseOffering
		var facID *int
		err := rows.Scan(&item.ID, &item.CourseID, &item.SemesterID, &facID, &item.MaxCapacity,
			&item.CourseCode, &item.CourseName, &item.Credits, &item.Department, &item.SemesterName,
			&item.FacultyName, &item.FacultyEmail)
		if err != nil {
			http.Error(w, `{"error":"Data scan error"}`, http.StatusInternalServerError)
			return
		}
		item.FacultyID = facID
		list = append(list, item)
	}

	json.NewEncoder(w).Encode(list)
}

func studentRegisterHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, _ := getClaimsFromContext(r)
	if claims.Role != "student" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Student access required"}`, http.StatusForbidden)
		return
	}

	var req struct {
		CourseOfferingID int `json:"course_offering_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CourseOfferingID <= 0 {
		http.Error(w, `{"error":"Invalid course offering ID"}`, http.StatusBadRequest)
		return
	}

	studentID := claims.RefID

	if useMemoryDB {
		memMutex.Lock()
		defer memMutex.Unlock()

		// Verify active semester registration open
		var isOpen bool
		for _, off := range memOfferings {
			if off.ID == req.CourseOfferingID {
				for _, sem := range memSemesters {
					if sem.ID == off.SemesterID && sem.IsActive && sem.RegistrationOpen {
						isOpen = true
						break
					}
				}
			}
		}
		if !isOpen {
			http.Error(w, `{"error":"Course registration is closed for this semester"}`, http.StatusBadRequest)
			return
		}

		// Check if already registered
		for _, reg := range memRegistrations {
			if reg.StudentID == studentID && reg.CourseOfferingID == req.CourseOfferingID {
				http.Error(w, `{"error":"You are already registered for this course"}`, http.StatusBadRequest)
				return
			}
		}

		newReg := CourseRegistration{
			ID:               registrationNextID,
			StudentID:        studentID,
			CourseOfferingID: req.CourseOfferingID,
			Status:           "registered",
			RegistrationDate: time.Now(),
		}
		registrationNextID++
		memRegistrations = append(memRegistrations, newReg)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":      "Successfully registered for course",
			"registration": newReg,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify active semester registration open
	var isOpen bool
	err := db.QueryRow(ctx, `
		SELECT s.registration_open
		FROM course_offerings co
		JOIN semesters s ON co.semester_id = s.id
		WHERE co.id = $1
	`, req.CourseOfferingID).Scan(&isOpen)

	if err != nil || !isOpen {
		http.Error(w, `{"error":"Course registration is closed for this semester"}`, http.StatusBadRequest)
		return
	}

	var regID int
	err = db.QueryRow(ctx, `
		INSERT INTO course_registrations (student_id, course_offering_id, status)
		VALUES ($1, $2, 'registered')
		RETURNING id
	`, studentID, req.CourseOfferingID).Scan(&regID)

	if err != nil {
		if strings.Contains(err.Error(), "unique_student_offering") {
			http.Error(w, `{"error":"You are already registered for this course"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"Failed to complete course registration"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Successfully registered for course",
		"id":      regID,
	})
}

// Faculty HTTP handlers

func facultyProfileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "faculty" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Faculty access required"}`, http.StatusForbidden)
		return
	}

	facultyID := claims.RefID
	var f Faculty
	var found bool

	if useMemoryDB {
		memMutex.RLock()
		for _, fac := range memFaculty {
			if fac.ID == facultyID {
				f = fac
				found = true
				break
			}
		}
		memMutex.RUnlock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := db.QueryRow(ctx, `
			SELECT id, faculty_id, name, email, phone, department, designation, created_at
			FROM faculty WHERE id = $1
		`, facultyID).Scan(&f.ID, &f.FacultyID, &f.Name, &f.Email, &f.Phone, &f.Department, &f.Designation, &f.CreatedAt)
		if err == nil {
			found = true
		}
	}

	if !found {
		http.Error(w, `{"error":"Faculty profile not found"}`, http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(f)
}

func facultyCoursesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "faculty" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Faculty access required"}`, http.StatusForbidden)
		return
	}

	facultyID := claims.RefID

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()

		var courses []CourseOffering
		for _, off := range memOfferings {
			if off.FacultyID != nil && *off.FacultyID == facultyID {
				item := off
				for _, c := range memCourses {
					if c.ID == off.CourseID {
						item.CourseCode = c.CourseID
						item.CourseName = c.CourseName
						item.Credits = c.Credits
						item.Department = c.Department
					}
				}
				for _, sem := range memSemesters {
					if sem.ID == off.SemesterID {
						item.SemesterName = sem.Name
						item.IsActive = sem.IsActive
					}
				}
				courses = append(courses, item)
			}
		}
		json.NewEncoder(w).Encode(courses)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT 
			co.id, co.course_id, co.semester_id, co.faculty_id, co.max_capacity,
			c.course_id, c.course_name, c.credits, c.department, s.name, s.is_active
		FROM course_offerings co
		JOIN courses c ON co.course_id = c.id
		JOIN semesters s ON co.semester_id = s.id
		WHERE co.faculty_id = $1
		ORDER BY s.id DESC, c.course_id ASC
	`, facultyID)
	if err != nil {
		http.Error(w, `{"error":"Failed to fetch faculty taught courses"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []CourseOffering{}
	for rows.Next() {
		var item CourseOffering
		var facID *int
		err := rows.Scan(&item.ID, &item.CourseID, &item.SemesterID, &facID, &item.MaxCapacity,
			&item.CourseCode, &item.CourseName, &item.Credits, &item.Department, &item.SemesterName, &item.IsActive)
		if err != nil {
			http.Error(w, `{"error":"Data scan error"}`, http.StatusInternalServerError)
			return
		}
		item.FacultyID = facID
		list = append(list, item)
	}

	json.NewEncoder(w).Encode(list)
}

func facultyEnrolledStudentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, _ := getClaimsFromContext(r)

	if claims.Role != "faculty" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Faculty access required"}`, http.StatusForbidden)
		return
	}

	offeringIDText := r.URL.Query().Get("course_offering_id")
	offeringID, err := strconv.Atoi(offeringIDText)
	if err != nil || offeringID <= 0 {
		http.Error(w, `{"error":"Valid course_offering_id parameter required"}`, http.StatusBadRequest)
		return
	}

	facultyID := claims.RefID

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()

		// Verify faculty teaches offering
		teaches := false
		for _, off := range memOfferings {
			if off.ID == offeringID && (off.FacultyID != nil && *off.FacultyID == facultyID || claims.Role == "admin") {
				teaches = true
				break
			}
		}
		if !teaches {
			http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
			return
		}

		type EnrolledStudent struct {
			StudentID     int     `json:"student_id"`
			StudentRollNo string  `json:"student_roll_no"`
			Name          string  `json:"name"`
			Email         string  `json:"email"`
			Phone         string  `json:"phone"`
			Grade         string  `json:"grade"`
			Marks         float64 `json:"marks"`
		}

		var enrolled []EnrolledStudent
		for _, reg := range memRegistrations {
			if reg.CourseOfferingID == offeringID {
				for _, st := range memStudents {
					if st.ID == reg.StudentID {
						item := EnrolledStudent{
							StudentID:     st.ID,
							StudentRollNo: st.StudentID,
							Name:          st.Name,
							Email:         st.Email,
							Phone:         st.Phone,
						}
						for _, res := range memResults {
							if res.StudentID == st.ID && res.CourseOfferingID == offeringID {
								item.Grade = res.Grade
								item.Marks = res.Marks
							}
						}
						enrolled = append(enrolled, item)
					}
				}
			}
		}
		json.NewEncoder(w).Encode(enrolled)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify faculty teaches offering
	var teachesCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM course_offerings WHERE id = $1 AND (faculty_id = $2 OR $3 = 'admin')", offeringID, facultyID, claims.Role).Scan(&teachesCount)
	if err != nil || teachesCount == 0 {
		http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
		return
	}

	rows, err := db.Query(ctx, `
		SELECT 
			s.id, s.student_id, s.name, s.email, COALESCE(s.phone, ''), COALESCE(r.grade, ''), COALESCE(r.marks::float8, 0.0)
		FROM course_registrations cr
		JOIN students s ON cr.student_id = s.id
		LEFT JOIN results r ON (r.student_id = cr.student_id AND r.course_offering_id = cr.course_offering_id)
		WHERE cr.course_offering_id = $1
		ORDER BY s.student_id ASC
	`, offeringID)

	if err != nil {
		http.Error(w, `{"error":"Failed to fetch enrolled students"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type EnrolledStudent struct {
		StudentID     int     `json:"student_id"`
		StudentRollNo string  `json:"student_roll_no"`
		Name          string  `json:"name"`
		Email         string  `json:"email"`
		Phone         string  `json:"phone"`
		Grade         string  `json:"grade"`
		Marks         float64 `json:"marks"`
	}

	var enrolled []EnrolledStudent
	for rows.Next() {
		var item EnrolledStudent
		err := rows.Scan(&item.StudentID, &item.StudentRollNo, &item.Name, &item.Email, &item.Phone, &item.Grade, &item.Marks)
		if err != nil {
			http.Error(w, `{"error":"Data scan error"}`, http.StatusInternalServerError)
			return
		}
		enrolled = append(enrolled, item)
	}

	json.NewEncoder(w).Encode(enrolled)
}

func facultyGradeUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, _ := getClaimsFromContext(r)
	if claims.Role != "faculty" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Faculty access required"}`, http.StatusForbidden)
		return
	}

	var req GradeUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON payload"}`, http.StatusBadRequest)
		return
	}

	req.StudentRollNo = strings.TrimSpace(req.StudentRollNo)
	req.Grade = strings.TrimSpace(req.Grade)
	req.Remarks = strings.TrimSpace(req.Remarks)

	if req.CourseOfferingID <= 0 || (req.StudentID <= 0 && req.StudentRollNo == "") || req.Grade == "" {
		http.Error(w, `{"error":"course_offering_id, student_id/student_roll_no, and grade are required"}`, http.StatusBadRequest)
		return
	}

	facultyID := claims.RefID

	if useMemoryDB {
		memMutex.Lock()
		defer memMutex.Unlock()

		// Verify faculty teaches offering
		var offFound *CourseOffering
		for i, off := range memOfferings {
			if off.ID == req.CourseOfferingID && (off.FacultyID != nil && *off.FacultyID == facultyID || claims.Role == "admin") {
				offFound = &memOfferings[i]
				break
			}
		}
		if offFound == nil {
			http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
			return
		}

		// Resolve student
		var targetStudent *Student
		for i, st := range memStudents {
			if st.ID == req.StudentID || strings.EqualFold(st.StudentID, req.StudentRollNo) {
				targetStudent = &memStudents[i]
				break
			}
		}

		if targetStudent == nil {
			http.Error(w, `{"error":"Student not found with provided ID/Roll Number"}`, http.StatusNotFound)
			return
		}

		// Course details
		courseCode := "CS101"
		courseName := "Course"
		for _, c := range memCourses {
			if c.ID == offFound.CourseID {
				courseCode = c.CourseID
				courseName = c.CourseName
			}
		}

		semName := "Fall 2026"
		for _, sem := range memSemesters {
			if sem.ID == offFound.SemesterID {
				semName = sem.Name
			}
		}

		// Create or update result
		updated := false
		for i, res := range memResults {
			if res.StudentID == targetStudent.ID && res.CourseOfferingID == req.CourseOfferingID {
				memResults[i].Marks = req.Marks
				memResults[i].Grade = req.Grade
				memResults[i].Remarks = req.Remarks
				memResults[i].UpdatedAt = time.Now()
				updated = true
				break
			}
		}

		if !updated {
			newRes := Result{
				ID:               resultNextID,
				StudentID:        targetStudent.ID,
				CourseOfferingID: req.CourseOfferingID,
				Marks:            req.Marks,
				Grade:            req.Grade,
				Remarks:          req.Remarks,
				SemesterName:     semName,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			}
			resultNextID++
			memResults = append(memResults, newRes)
		}

		// Send email notification
		sendGradeNotification(targetStudent.Email, targetStudent.Name, courseCode, courseName, req.Grade, req.Marks, semName)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Grade uploaded successfully and email notification dispatched",
			"status":  "success",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify faculty teaches offering
	var courseCode, courseName, semName string
	err := db.QueryRow(ctx, `
		SELECT c.course_id, c.course_name, s.name
		FROM course_offerings co
		JOIN courses c ON co.course_id = c.id
		JOIN semesters s ON co.semester_id = s.id
		WHERE co.id = $1 AND (co.faculty_id = $2 OR $3 = 'admin')
	`, req.CourseOfferingID, facultyID, claims.Role).Scan(&courseCode, &courseName, &semName)

	if err != nil {
		http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
		return
	}

	// Resolve student
	var targetStudentID int
	var studentName, studentEmail string
	if req.StudentID > 0 {
		err = db.QueryRow(ctx, "SELECT id, name, email FROM students WHERE id = $1", req.StudentID).Scan(&targetStudentID, &studentName, &studentEmail)
	} else {
		err = db.QueryRow(ctx, "SELECT id, name, email FROM students WHERE LOWER(student_id) = LOWER($1)", req.StudentRollNo).Scan(&targetStudentID, &studentName, &studentEmail)
	}

	if err != nil {
		http.Error(w, `{"error":"Student not found with provided ID/Roll Number"}`, http.StatusNotFound)
		return
	}

	// UPSERT result
	_, err = db.Exec(ctx, `
		INSERT INTO results (student_id, course_offering_id, marks, grade, remarks, semester_name, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (student_id, course_offering_id)
		DO UPDATE SET marks = EXCLUDED.marks, grade = EXCLUDED.grade, remarks = EXCLUDED.remarks, updated_at = CURRENT_TIMESTAMP
	`, targetStudentID, req.CourseOfferingID, req.Marks, req.Grade, req.Remarks, semName)

	if err != nil {
		log.Println("Grade save DB error:", err)
		http.Error(w, fmt.Sprintf(`{"error":"Failed to save grade to database: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Trigger email
	sendGradeNotification(studentEmail, studentName, courseCode, courseName, req.Grade, req.Marks, semName)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Grade uploaded successfully and email notification dispatched",
		"status":  "success",
	})
}

func facultyBulkGradeUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, _ := getClaimsFromContext(r)
	if claims.Role != "faculty" && claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Faculty access required"}`, http.StatusForbidden)
		return
	}

	contentType := r.Header.Get("Content-Type")

	var courseOfferingID int
	var rowsToProcess []BulkGradeRow

	if strings.Contains(contentType, "text/csv") || strings.Contains(contentType, "multipart/form-data") {
		// Handle CSV multipart or raw upload
		r.ParseMultipartForm(10 << 20)
		offeringStr := r.FormValue("course_offering_id")
		courseOfferingID, _ = strconv.Atoi(offeringStr)

		var reader *csv.Reader
		file, _, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			reader = csv.NewReader(file)
		} else {
			// Try reading raw body as CSV
			bodyBytes, _ := io.ReadAll(r.Body)
			reader = csv.NewReader(bytes.NewReader(bodyBytes))
		}

		records, err := reader.ReadAll()
		if err != nil || len(records) < 2 {
			http.Error(w, `{"error":"Invalid CSV file or empty data"}`, http.StatusBadRequest)
			return
		}

		// Assume header row 0: student_roll_no, marks, grade, remarks
		for i := 1; i < len(records); i++ {
			rec := records[i]
			if len(rec) >= 3 {
				roll := strings.TrimSpace(rec[0])
				marks, _ := strconv.ParseFloat(strings.TrimSpace(rec[1]), 64)
				grd := strings.TrimSpace(rec[2])
				rem := ""
				if len(rec) >= 4 {
					rem = strings.TrimSpace(rec[3])
				}
				rowsToProcess = append(rowsToProcess, BulkGradeRow{
					StudentRollNo: roll,
					Marks:         marks,
					Grade:         grd,
					Remarks:       rem,
				})
			}
		}
	} else {
		// Handle JSON payload
		var req BulkGradeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid JSON payload"}`, http.StatusBadRequest)
			return
		}
		courseOfferingID = req.CourseOfferingID
		rowsToProcess = req.Grades
	}

	if courseOfferingID <= 0 || len(rowsToProcess) == 0 {
		http.Error(w, `{"error":"course_offering_id and at least one grade row are required"}`, http.StatusBadRequest)
		return
	}

	facultyID := claims.RefID
	var successCount int
	var failedRows []BulkFailedRow

	if useMemoryDB {
		memMutex.Lock()
		defer memMutex.Unlock()

		// Verify faculty teaches offering
		var offFound *CourseOffering
		for i, off := range memOfferings {
			if off.ID == courseOfferingID && (off.FacultyID != nil && *off.FacultyID == facultyID || claims.Role == "admin") {
				offFound = &memOfferings[i]
				break
			}
		}
		if offFound == nil {
			http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
			return
		}

		courseCode := "CS101"
		courseName := "Course"
		for _, c := range memCourses {
			if c.ID == offFound.CourseID {
				courseCode = c.CourseID
				courseName = c.CourseName
			}
		}

		semName := "Fall 2026"
		for _, sem := range memSemesters {
			if sem.ID == offFound.SemesterID {
				semName = sem.Name
			}
		}

		for idx, row := range rowsToProcess {
			roll := strings.TrimSpace(row.StudentRollNo)
			if roll == "" {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Missing student roll number"})
				continue
			}

			var targetStudent *Student
			for i, st := range memStudents {
				if strings.EqualFold(st.StudentID, roll) {
					targetStudent = &memStudents[i]
					break
				}
			}

			if targetStudent == nil {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Unknown roll number - student not found"})
				continue
			}

			if row.Grade == "" {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Missing letter grade"})
				continue
			}

			// Save result
			updated := false
			for i, res := range memResults {
				if res.StudentID == targetStudent.ID && res.CourseOfferingID == courseOfferingID {
					memResults[i].Marks = row.Marks
					memResults[i].Grade = row.Grade
					memResults[i].Remarks = row.Remarks
					memResults[i].UpdatedAt = time.Now()
					updated = true
					break
				}
			}

			if !updated {
				newRes := Result{
					ID:               resultNextID,
					StudentID:        targetStudent.ID,
					CourseOfferingID: courseOfferingID,
					Marks:            row.Marks,
					Grade:            row.Grade,
					Remarks:          row.Remarks,
					SemesterName:     semName,
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
				}
				resultNextID++
				memResults = append(memResults, newRes)
			}

			sendGradeNotification(targetStudent.Email, targetStudent.Name, courseCode, courseName, row.Grade, row.Marks, semName)
			successCount++
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var courseCode, courseName, semName string
		err := db.QueryRow(ctx, `
			SELECT c.course_id, c.course_name, s.name
			FROM course_offerings co
			JOIN courses c ON co.course_id = c.id
			JOIN semesters s ON co.semester_id = s.id
			WHERE co.id = $1 AND (co.faculty_id = $2 OR $3 = 'admin')
		`, courseOfferingID, facultyID, claims.Role).Scan(&courseCode, &courseName, &semName)

		if err != nil {
			http.Error(w, `{"error":"Unauthorized: You do not teach this course offering"}`, http.StatusForbidden)
			return
		}

		for idx, row := range rowsToProcess {
			roll := strings.TrimSpace(row.StudentRollNo)
			if roll == "" {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Missing student roll number"})
				continue
			}

			var targetStudentID int
			var studentName, studentEmail string
			err := db.QueryRow(ctx, "SELECT id, name, email FROM students WHERE LOWER(student_id) = LOWER($1)", roll).Scan(&targetStudentID, &studentName, &studentEmail)

			if err != nil {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Unknown roll number - student not found"})
				continue
			}

			if row.Grade == "" {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Missing letter grade"})
				continue
			}

			_, err = db.Exec(ctx, `
				INSERT INTO results (student_id, course_offering_id, marks, grade, remarks, semester_name, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
				ON CONFLICT (student_id, course_offering_id)
				DO UPDATE SET marks = EXCLUDED.marks, grade = EXCLUDED.grade, remarks = EXCLUDED.remarks, updated_at = CURRENT_TIMESTAMP
			`, targetStudentID, courseOfferingID, row.Marks, row.Grade, row.Remarks, semName)

			if err != nil {
				failedRows = append(failedRows, BulkFailedRow{Row: idx + 1, StudentRollNo: roll, Reason: "Database write error"})
				continue
			}

			sendGradeNotification(studentEmail, studentName, courseCode, courseName, row.Grade, row.Marks, semName)
			successCount++
		}
	}

	json.NewEncoder(w).Encode(BulkGradeResponse{
		SuccessCount: successCount,
		FailedCount:  len(failedRows),
		FailedRows:   failedRows,
		Message:      fmt.Sprintf("Bulk grade processing complete: %d saved, %d failed", successCount, len(failedRows)),
	})
}

// System & utility handlers

func emailLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()
		json.NewEncoder(w).Encode(memEmailLogs)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT id, recipient_email, student_name, subject, body, status, sent_at
		FROM email_logs ORDER BY id DESC LIMIT 50
	`)
	if err != nil {
		http.Error(w, `{"error":"Failed to fetch email logs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	logs := []EmailLog{}
	for rows.Next() {
		var l EmailLog
		err := rows.Scan(&l.ID, &l.RecipientEmail, &l.StudentName, &l.Subject, &l.Body, &l.Status, &l.SentAt)
		if err != nil {
			http.Error(w, `{"error":"Scan error"}`, http.StatusInternalServerError)
			return
		}
		logs = append(logs, l)
	}

	json.NewEncoder(w).Encode(logs)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mode := "PostgreSQL"
	if useMemoryDB {
		mode = "In-Memory Fallback"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "AMS Backend API operational (" + mode + ")",
		"mode":    mode,
		"time":    time.Now().Format(time.RFC3339),
	})
}

func dbHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if useMemoryDB {
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Running in-memory database mode",
			"mode":    "memory",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "PostgreSQL connection failed",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "PostgreSQL connection healthy",
		"mode":    "postgresql",
	})
}

// CORS middleware

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func adminStudentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, ok := getClaimsFromContext(r)
	if !ok || claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Admin access required"}`, http.StatusForbidden)
		return
	}

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()
		json.NewEncoder(w).Encode(memStudents)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `SELECT id, student_id, name, email, phone, department, program, year, section, created_at FROM students ORDER BY id ASC`)
	if err != nil {
		http.Error(w, `{"error":"Database query error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	students := []Student{}
	for rows.Next() {
		var s Student
		if err := rows.Scan(&s.ID, &s.StudentID, &s.Name, &s.Email, &s.Phone, &s.Department, &s.Program, &s.Year, &s.Section, &s.CreatedAt); err == nil {
			students = append(students, s)
		}
	}
	json.NewEncoder(w).Encode(students)
}

func adminFacultyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, ok := getClaimsFromContext(r)
	if !ok || claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Admin access required"}`, http.StatusForbidden)
		return
	}

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()
		json.NewEncoder(w).Encode(memFaculty)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `SELECT id, faculty_id, name, email, phone, department, designation, created_at FROM faculty ORDER BY id ASC`)
	if err != nil {
		http.Error(w, `{"error":"Database query error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	facultyList := []Faculty{}
	for rows.Next() {
		var f Faculty
		if err := rows.Scan(&f.ID, &f.FacultyID, &f.Name, &f.Email, &f.Phone, &f.Department, &f.Designation, &f.CreatedAt); err == nil {
			facultyList = append(facultyList, f)
		}
	}
	json.NewEncoder(w).Encode(facultyList)
}

func adminCoursesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, ok := getClaimsFromContext(r)
	if !ok || claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Admin access required"}`, http.StatusForbidden)
		return
	}

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()

		var courses []CourseOffering
		for _, off := range memOfferings {
			item := off
			for _, c := range memCourses {
				if c.ID == off.CourseID {
					item.CourseCode = c.CourseID
					item.CourseName = c.CourseName
					item.Credits = c.Credits
					item.Department = c.Department
				}
			}
			for _, sem := range memSemesters {
				if sem.ID == off.SemesterID {
					item.SemesterName = sem.Name
					item.IsActive = sem.IsActive
				}
			}
			if off.FacultyID != nil {
				for _, f := range memFaculty {
					if f.ID == *off.FacultyID {
						item.FacultyName = f.Name
					}
				}
			} else {
				item.FacultyName = "Unassigned"
			}
			courses = append(courses, item)
		}
		json.NewEncoder(w).Encode(courses)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT 
			co.id, co.course_id, co.semester_id, co.faculty_id, co.max_capacity,
			c.course_id, c.course_name, c.credits, c.department, s.name, s.is_active, COALESCE(f.name, 'Unassigned')
		FROM course_offerings co
		JOIN courses c ON co.course_id = c.id
		JOIN semesters s ON co.semester_id = s.id
		LEFT JOIN faculty f ON co.faculty_id = f.id
		ORDER BY s.id DESC, c.course_id ASC
	`)
	if err != nil {
		http.Error(w, `{"error":"Database query error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []CourseOffering{}
	for rows.Next() {
		var item CourseOffering
		var facID *int
		err := rows.Scan(&item.ID, &item.CourseID, &item.SemesterID, &facID, &item.MaxCapacity,
			&item.CourseCode, &item.CourseName, &item.Credits, &item.Department, &item.SemesterName, &item.IsActive, &item.FacultyName)
		if err == nil {
			item.FacultyID = facID
			list = append(list, item)
		} else {
			log.Println("adminCoursesHandler scan error:", err)
		}
	}
	json.NewEncoder(w).Encode(list)
}

func adminStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, ok := getClaimsFromContext(r)
	if !ok || claims.Role != "admin" {
		http.Error(w, `{"error":"Forbidden: Admin access required"}`, http.StatusForbidden)
		return
	}

	if useMemoryDB {
		memMutex.RLock()
		defer memMutex.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_students":   len(memStudents),
			"total_faculty":    len(memFaculty),
			"total_courses":    len(memCourses),
			"total_offerings":  len(memOfferings),
			"active_semester":  "Semester 2",
			"total_email_logs": len(memEmailLogs),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stuCount, facCount, crsCount, offCount, logCount int
	var activeSem string
	db.QueryRow(ctx, "SELECT COUNT(*) FROM students").Scan(&stuCount)
	db.QueryRow(ctx, "SELECT COUNT(*) FROM faculty").Scan(&facCount)
	db.QueryRow(ctx, "SELECT COUNT(*) FROM courses").Scan(&crsCount)
	db.QueryRow(ctx, "SELECT COUNT(*) FROM course_offerings").Scan(&offCount)
	db.QueryRow(ctx, "SELECT COUNT(*) FROM email_logs").Scan(&logCount)
	db.QueryRow(ctx, "SELECT name FROM semesters WHERE is_active = true LIMIT 1").Scan(&activeSem)
	if activeSem == "" {
		activeSem = "Semester 2"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_students":   stuCount,
		"total_faculty":    facCount,
		"total_courses":    crsCount,
		"total_offerings":  offCount,
		"active_semester":  activeSem,
		"total_email_logs": logCount,
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	mode := "PostgreSQL"
	if useMemoryDB {
		mode = "In-Memory Fallback"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "operational",
		"service": "AMS Academic Management System Backend API",
		"mode":    mode,
		"health":  "/api/health",
		"time":    time.Now().Format(time.RFC3339),
	})
}

// Server entrypoint

func main() {
	port := getEnv("PORT", "8080")
	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {
		password := getEnv("AMS_DB_PASSWORD", "postgres")
		dbURL = fmt.Sprintf("postgres://postgres:%s@localhost:5432/ams", password)
	}

	initInMemoryStore()

	config, err := pgxpool.ParseConfig(dbURL)
	if err == nil {
		initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer initCancel()

		pool, errConnect := pgxpool.NewWithConfig(initCtx, config)
		if errConnect == nil && pool.Ping(initCtx) == nil {
			db = pool
			defer db.Close()
			log.Println("Connected to PostgreSQL database pool successfully.")
			if errInit := initTables(initCtx); errInit != nil {
				log.Println("PostgreSQL table init error:", errInit)
			} else {
				useMemoryDB = false
				log.Println("Unconditionally running database seed & sync in PostgreSQL...")
				seedDatabaseIfEmpty(initCtx)
			}
		} else {
			useMemoryDB = true
			if errConnect != nil {
				log.Println("Notice: PostgreSQL connect error:", errConnect)
			}
			log.Println("Notice: PostgreSQL unavailable. Active in in-memory fallback server mode.")
		}
	} else {
		useMemoryDB = true
		log.Println("Notice: No PostgreSQL database configured. Active in in-memory fallback server mode.")
	}

	mux := http.NewServeMux()

	// Public / Health Routes
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/db-health", dbHealthHandler)
	mux.HandleFunc("/api/auth/login", loginHandler)
	mux.HandleFunc("/api/auth/me", authMiddleware(meHandler))
	mux.HandleFunc("/api/email-logs", emailLogsHandler)

	// Student Routes
	mux.HandleFunc("/api/student/profile", authMiddleware(studentProfileHandler))
	mux.HandleFunc("/api/student/courses", authMiddleware(studentCoursesHandler))
	mux.HandleFunc("/api/student/available-courses", authMiddleware(studentAvailableCoursesHandler))
	mux.HandleFunc("/api/student/register", authMiddleware(studentRegisterHandler))

	// Faculty Routes
	mux.HandleFunc("/api/faculty/profile", authMiddleware(facultyProfileHandler))
	mux.HandleFunc("/api/faculty/courses", authMiddleware(facultyCoursesHandler))
	mux.HandleFunc("/api/faculty/enrolled-students", authMiddleware(facultyEnrolledStudentsHandler))
	mux.HandleFunc("/api/faculty/grade", authMiddleware(facultyGradeUploadHandler))
	mux.HandleFunc("/api/faculty/bulk-grades", authMiddleware(facultyBulkGradeUploadHandler))

	// Admin Routes
	mux.HandleFunc("/api/admin/students", authMiddleware(adminStudentsHandler))
	mux.HandleFunc("/api/admin/faculty", authMiddleware(adminFacultyHandler))
	mux.HandleFunc("/api/admin/courses", authMiddleware(adminCoursesHandler))
	mux.HandleFunc("/api/admin/stats", authMiddleware(adminStatsHandler))

	log.Printf("AMS Backend Server listening on port %s", port)
	handler := corsMiddleware(mux)

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}
