package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type Task struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID       int
	Username string
}

var db *sql.DB

func main() {
	var err error

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to PostgreSQL")

	createTablesIfNotExists()

	http.HandleFunc("/signup", signup)
	http.HandleFunc("/login", login)

	http.HandleFunc("/tasks", getTasks)
	http.HandleFunc("/add", addTask)
	http.HandleFunc("/delete", deleteTask)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// 🔐 CREATE TABLES
func createTablesIfNotExists() {
	userTable := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL
	);`

	taskTable := `
	CREATE TABLE IF NOT EXISTS tasks (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		user_id INT REFERENCES users(id) ON DELETE CASCADE
	);`

	db.Exec(userTable)
	db.Exec(taskTable)
}

// 🆕 SIGNUP
func signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Error(w, "Username and password required", 400)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error hashing password", 500)
		return
	}

	_, err = db.Exec(
		"INSERT INTO users (username, password_hash) VALUES ($1, $2)",
		username,
		string(hash),
	)

	if err != nil {
		http.Error(w, "Username already exists", 400)
		return
	}

	fmt.Fprintln(w, "User created successfully")
}

// 🔓 LOGIN
func login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var userID int
	var storedHash string

	err := db.QueryRow(
		"SELECT id, password_hash FROM users WHERE username=$1",
		username,
	).Scan(&userID, &storedHash)

	if err != nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(storedHash),
		[]byte(password),
	)

	if err != nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	json.NewEncoder(w).Encode(User{
		ID:       userID,
		Username: username,
	})
}

// 📄 GET TASKS (TEMP: shows all tasks – will secure later)
func getTasks(w http.ResponseWriter, r *http.Request) {
	rows, _ := db.Query("SELECT id, name FROM tasks")
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.Name)
		tasks = append(tasks, t)
	}

	json.NewEncoder(w).Encode(tasks)
}

// ➕ ADD TASK
func addTask(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "Task name required", 400)
		return
	}

	db.Exec("INSERT INTO tasks (name) VALUES ($1)", name)
	fmt.Fprintln(w, "Task added")
}

// ❌ DELETE TASK
func deleteTask(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	db.Exec("DELETE FROM tasks WHERE id=$1", id)
	fmt.Fprintln(w, "Task deleted")
}
