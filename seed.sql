-- Seed data for Academic Management System (AMS)
-- Passwords for default users: password123

-- 1. STUDENTS (5 Members - Simple Common Names)
INSERT INTO students (id, student_id, name, email, phone, department, program, year, section) VALUES
(1, 'STU101', 'Priya Sharma', 'priya@university.edu', '555-0101', 'Computer Science', 'B.Tech Computer Science', 2, 'A'),
(2, 'STU102', 'Meenu Patel', 'meenu@university.edu', '555-0102', 'Electrical Engineering', 'B.Tech Electrical Engineering', 3, 'B'),
(3, 'STU103', 'Ananya Reddy', 'ananya@university.edu', '555-0103', 'Computer Science', 'B.Tech Computer Science', 1, 'A'),
(4, 'STU104', 'Karthik Kumar', 'karthik@university.edu', '555-0104', 'Information Technology', 'B.Tech IT', 4, 'C'),
(5, 'STU105', 'Rahul Verma', 'rahul@university.edu', '555-0105', 'Computer Science', 'B.Tech AI & ML', 2, 'B')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, student_id = EXCLUDED.student_id;

-- 2. FACULTY (5 Members)
INSERT INTO faculty (id, faculty_id, name, email, phone, department, designation) VALUES
(1, 'FAC201', 'Dr. K. Seshadri', 'seshadri@university.edu', '555-0201', 'Computer Science', 'Professor'),
(2, 'FAC202', 'Dr. Meenakshi', 'meenakshi@university.edu', '555-0202', 'Electrical Engineering', 'Associate Professor'),
(3, 'FAC203', 'Dr. N. Ramaswamy', 'ramaswamy@university.edu', '555-0203', 'Mechanical Engineering', 'Professor'),
(4, 'FAC204', 'Dr. Radhakrishnan', 'radhakrishnan@university.edu', '555-0204', 'Information Technology', 'Associate Professor'),
(5, 'FAC205', 'Dr. C. V. Raman', 'raman@university.edu', '555-0205', 'Computer Science', 'Assistant Professor')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, faculty_id = EXCLUDED.faculty_id;

-- 3. SEMESTERS
INSERT INTO semesters (id, name, code, is_active, registration_open) VALUES
(1, 'Semester 1', 'SEM1', false, false),
(2, 'Semester 2', 'SEM2', true, true)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, is_active = EXCLUDED.is_active;

-- 4. COURSES (11 Courses)
INSERT INTO courses (id, course_id, course_name, department, credits, description) VALUES
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
ON CONFLICT (id) DO UPDATE SET course_name = EXCLUDED.course_name, department = EXCLUDED.department;

-- 5. COURSE OFFERINGS (Active & Historical)
INSERT INTO course_offerings (id, course_id, semester_id, faculty_id, max_capacity) VALUES
(1, 1, 2, 1, 60), -- CS101 in Semester 2 by Dr. K. Seshadri
(2, 2, 2, 2, 60), -- EE201 in Semester 2 by Dr. Meenakshi
(3, 3, 1, 1, 60), -- CS202 in Semester 1 by Dr. K. Seshadri
(4, 4, 1, 2, 60), -- EE301 in Semester 1 by Dr. Meenakshi
(5, 5, 2, 3, 60), -- ME101 in Semester 2 by Dr. N. Ramaswamy
(6, 6, 2, 4, 60), -- IT201 in Semester 2 by Dr. Radhakrishnan
(7, 7, 2, 1, 60), -- AI301 in Semester 2 by Dr. K. Seshadri
(8, 8, 2, 4, 60), -- CYB302 in Semester 2 by Dr. Radhakrishnan
(9, 9, 2, 4, 60), -- CYB401 in Semester 2 by Dr. Radhakrishnan
(10, 10, 2, 5, 60), -- AI402 in Semester 2 by Dr. C. V. Raman
(11, 11, 2, 4, 60)  -- IT305 in Semester 2 by Dr. Radhakrishnan
ON CONFLICT (id) DO UPDATE SET faculty_id = EXCLUDED.faculty_id;

-- 6. COURSE REGISTRATIONS
INSERT INTO course_registrations (id, student_id, course_offering_id, status) VALUES
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
ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status;

-- 7. RESULTS / GRADES
INSERT INTO results (id, student_id, course_offering_id, marks, grade, remarks, semester_name) VALUES
(1, 1, 3, 95.0, 'A+', 'Outstanding performance in algorithms', 'Semester 1'),
(2, 2, 4, 88.0, 'A', 'Excellent lab coursework and exam result', 'Semester 1')
ON CONFLICT (id) DO UPDATE SET marks = EXCLUDED.marks, grade = EXCLUDED.grade;

-- 8. USERS
INSERT INTO users (id, username, email, password_hash, role, ref_id) VALUES
(1, 'STU101', 'priya@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'student', 1),
(2, 'STU102', 'meenu@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'student', 2),
(3, 'STU103', 'ananya@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'student', 3),
(4, 'STU104', 'karthik@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'student', 4),
(5, 'STU105', 'rahul@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'student', 5),
(6, 'FAC201', 'seshadri@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'faculty', 1),
(7, 'FAC202', 'meenakshi@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'faculty', 2),
(8, 'FAC203', 'ramaswamy@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'faculty', 3),
(9, 'FAC204', 'radhakrishnan@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'faculty', 4),
(10, 'FAC205', 'raman@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'faculty', 5),
(11, 'admin', 'admin@university.edu', '$2a$10$425XF9O/v5K1X0lQe.0dneW1d15QZ87Xp9J4U.U1E9v0J8O8wL6kK', 'admin', 0)
ON CONFLICT (username) DO UPDATE SET email = EXCLUDED.email;
