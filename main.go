package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int
	Username string
}

type Task struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
}

var db *sql.DB
var jwtSecret []byte

func main() {
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		log.Fatal("JWT_SECRET not set")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to PostgreSQL")
	createTables()

	// AUTH
	http.HandleFunc("/signup", signup)
	http.HandleFunc("/login", login)

	// TASKS (JWT protected)
	http.HandleFunc("/tasks", auth(getTasks))
	http.HandleFunc("/add", auth(addTask))
	http.HandleFunc("/toggle", auth(toggleTask))
	http.HandleFunc("/delete", auth(deleteTask))

	// FRONTEND - Serve static.html at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "./static/static.html")
		} else {
			http.NotFound(w, r)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

//////////////////////////////////////////////////
// DATABASE
//////////////////////////////////////////////////

func createTables() {
	db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL
	)`)

	db.Exec(`
	CREATE TABLE IF NOT EXISTS tasks (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		completed BOOLEAN DEFAULT FALSE,
		user_id INT REFERENCES users(id) ON DELETE CASCADE
	)`)
	
	// Migration: Add completed column if it doesn't exist
	db.Exec(`
	DO $ 
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM information_schema.columns 
			WHERE table_name='tasks' AND column_name='completed'
		) THEN
			ALTER TABLE tasks ADD COLUMN completed BOOLEAN DEFAULT FALSE;
		END IF;
	END $;
	`)
}

//////////////////////////////////////////////////
// AUTH
//////////////////////////////////////////////////

func signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Error(w, "Missing fields", 400)
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	_, err := db.Exec(
		"INSERT INTO users (username, password_hash) VALUES ($1, $2)",
		username, string(hash),
	)

	if err != nil {
		http.Error(w, "Username exists", 400)
		return
	}

	fmt.Fprintln(w, "Signup successful")
}

func login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var userID int
	var hash string

	err := db.QueryRow(
		"SELECT id, password_hash FROM users WHERE username=$1",
		username,
	).Scan(&userID, &hash)

	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenStr, _ := token.SignedString(jwtSecret)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenStr,
	})
}

//////////////////////////////////////////////////
// AUTH MIDDLEWARE
//////////////////////////////////////////////////

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "Unauthorized", 401)
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", 401)
			return
		}

		claims := token.Claims.(jwt.MapClaims)
		userID := int(claims["user_id"].(float64))

		ctx := context.WithValue(r.Context(), "user_id", userID)
		next(w, r.WithContext(ctx))
	}
}

//////////////////////////////////////////////////
// TASKS
//////////////////////////////////////////////////

func getTasks(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	filter := r.URL.Query().Get("filter")

	query := "SELECT id, name, completed FROM tasks WHERE user_id=$1"
	if filter == "active" {
		query += " AND completed=false"
	} else if filter == "completed" {
		query += " AND completed=true"
	}

	rows, err := db.Query(query+" ORDER BY id", userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.Name, &t.Completed)
		tasks = append(tasks, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func addTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "Task name required", 400)
		return
	}

	_, err := db.Exec("INSERT INTO tasks (name, user_id) VALUES ($1, $2)", name, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	
	w.WriteHeader(http.StatusOK)
}

func toggleTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))

	_, err := db.Exec("UPDATE tasks SET completed = NOT completed WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	
	w.WriteHeader(http.StatusOK)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))

	_, err := db.Exec("DELETE FROM tasks WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	
	w.WriteHeader(http.StatusOK)
}
