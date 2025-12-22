package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	_ "github.com/lib/pq"
)

type Task struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var db *sql.DB

func main() {
	var err error

	// 🔹 Update password if needed
	db, err = sql.Open("postgres",
		"host=localhost port=5432 user=postgres password=Lalith@123psgre dbname=todo_app sslmode=disable")
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	fmt.Println("Connected to PostgreSQL")

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/tasks", getTasks)
	http.HandleFunc("/add", addTask)
	http.HandleFunc("/delete", deleteTask)

	fmt.Println("Server running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

// 🔹 GET all tasks
func getTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name FROM tasks ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.Name)
		tasks = append(tasks, t)
	}

	json.NewEncoder(w).Encode(tasks)
}

// 🔹 ADD a task
func addTask(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Task name required", http.StatusBadRequest)
		return
	}

	var id int
	err := db.QueryRow(
		"INSERT INTO tasks (name) VALUES ($1) RETURNING id",
		name,
	).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(Task{ID: id, Name: name})
}

// 🔹 DELETE a task
func deleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	result, err := db.Exec("DELETE FROM tasks WHERE id=$1", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	fmt.Fprintln(w, "Task deleted")
}
