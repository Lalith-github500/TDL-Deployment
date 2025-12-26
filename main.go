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
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type Task struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
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

	http.HandleFunc("/tasks", authMiddleware(getTasks))
	http.HandleFunc("/add", authMiddleware(addTask))
	http.HandleFunc("/delete", authMiddleware(deleteTask))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// table creation
func createTablesIfNotExists() {
	db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL
	);
	`)

	db.Exec(`
	CREATE TABLE IF NOT EXISTS tasks (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		user_id INT REFERENCES users(id) ON DELETE CASCADE
	);
	`)
}

// signup
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

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	_, err := db.Exec(
		"INSERT INTO users (username, password_hash) VALUES ($1, $2)",
		username, string(hash),
	)

	if err != nil {
		http.Error(w, "Username already exists", 400)
		return
	}

	fmt.Fprintln(w, "User created")
}

// login
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

	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) != nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenStr, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenStr,
	})
}

// authorization middleware
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			http.Error(w, "Unauthorized", 401)
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
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

// tasks
func getTasks(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)

	rows, _ := db.Query(
		"SELECT id, name FROM tasks WHERE user_id=$1",
		userID,
	)
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.Name)
		tasks = append(tasks, t)
	}

	json.NewEncoder(w).Encode(tasks)
}

func addTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "Task name required", 400)
		return
	}

	db.Exec(
		"INSERT INTO tasks (name, user_id) VALUES ($1, $2)",
		name, userID,
	)

	fmt.Fprintln(w, "Task added")
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))

	db.Exec(
		"DELETE FROM tasks WHERE id=$1 AND user_id=$2",
		id, userID,
	)

	fmt.Fprintln(w, "Task deleted")
}
