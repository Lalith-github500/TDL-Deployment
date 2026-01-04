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
)

type Task struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
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

	createTableIfNotExists()

	// API routes
	http.HandleFunc("/tasks", getTasks)
	http.HandleFunc("/add", addTask)
	http.HandleFunc("/toggle", toggleTask)
	http.HandleFunc("/delete", deleteTask)

	// Serve frontend
	http.Handle("/", http.FileServer(http.Dir("./static")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// CREATE TABLE
func createTableIfNotExists() {
	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		completed BOOLEAN DEFAULT FALSE
	);`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}

// GET TASKS
// /tasks?filter=all | active | completed
func getTasks(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")

	query := "SELECT id, name, completed FROM tasks ORDER BY id"

	if filter == "active" {
		query = "SELECT id, name, completed FROM tasks WHERE completed = false ORDER BY id"
	} else if filter == "completed" {
		query = "SELECT id, name, completed FROM tasks WHERE completed = true ORDER BY id"
	}

	rows, err := db.Query(query)
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

	json.NewEncoder(w).Encode(tasks)
}

// ADD TASK
func addTask(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	if name == "" {
		http.Error(w, "Task name required", 400)
		return
	}

	_, err := db.Exec("INSERT INTO tasks (name) VALUES ($1)", name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintln(w, "Task added")
}

// TOGGLE COMPLETE
func toggleTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", 400)
		return
	}

	_, err = db.Exec(
		"UPDATE tasks SET completed = NOT completed WHERE id = $1",
		id,
	)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintln(w, "Task updated")
}

// DELETE TASK
func deleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", 400)
		return
	}

	_, err = db.Exec("DELETE FROM tasks WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintln(w, "Task deleted")
}
