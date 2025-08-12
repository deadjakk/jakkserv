package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/ini.v1"
)

var cfg *ini.File

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", cfg.Section("general").Key("database").String())
	if err != nil {
		panic(err)
	}
	// Create table if it doesn't exist
	createTableSQL := `CREATE TABLE IF NOT EXISTS entries (id INTEGER PRIMARY KEY AUTOINCREMENT, tag TEXT UNIQUE, url TEXT);`
	db.Exec(createTableSQL)
}

func ipHandler(w http.ResponseWriter, r *http.Request) {
	sAddr := strings.Split(r.RemoteAddr, ":")
	addr := strings.Join(sAddr[:len(sAddr)-1], ":")
	fmt.Fprintf(w, "%s", addr)
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var params struct {
		Level string `json:"level"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if params.Level == "" || params.Body == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	recipients := []string{}
	for value := range strings.SplitSeq(cfg.Section("smtp").Key("sendto").String(), ",") {
		trimmedValue := strings.TrimSpace(value)
		recipients = append(recipients, trimmedValue)
	}

	auth := smtp.PlainAuth("", cfg.Section("smtp").Key("username").String(), cfg.Section("smtp").Key("password").String(), cfg.Section("smtp").Key("server").String())
	err := smtp.SendMail(
		cfg.Section("smtp").Key("server").String()+":"+cfg.Section("smtp").Key("port").String(),
		auth,
		cfg.Section("smtp").Key("username").String(),
		recipients,
		[]byte(params.Level+"\r\n\r\n"+params.Body),
	)

	if err != nil {
		http.Error(w, "RequestFailed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, "success")
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var params struct {
		Tag string `json:"tag"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO entries (tag, url) VALUES (?, ?)", params.Tag, params.URL)
	if err != nil {
		http.Error(w, "Failed to save data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, "success")
}

func retrieveHandler(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		http.Error(w, "Tag is required", http.StatusBadRequest)
		return
	}

	var url string
	err := db.QueryRow("SELECT url FROM entries WHERE tag = ?", tag).Scan(&url)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No URL found for the given tag", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve data: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func parseConfig() bool {
	var err error
	cfg, err = ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Failed to load config file: %v", err)
		return false
	}
	smtpKeys := []string{"server", "port", "username", "password", "sendto"}
	if !checkConfig("smtp", smtpKeys) {
		return false
	}
	if !checkConfig("general", []string{
		"database", "secret", "authheader", "sslport",
		"httpport", "sslcert", "sslkey", "httpenabled",
		"sslenabled",
	}) {
		return false
	}

	return true
}

func authWrapper(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get(cfg.Section("general").Key("authheader").String())
		if auth == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if auth != cfg.Section("general").Key("secret").String() {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func checkConfig(section string, keys []string) bool {
	if cfg.Section(section) == nil {
		fmt.Printf("Missing config section: %s", section)
		return false
	}
	for _, value := range keys {
		if cfg.Section(section).Key(value).String() == "" {
			fmt.Printf("Missing key: '%s' under section: %s\n", value, section)
			return false
		}
	}
	return true
}

func main() {
	if !parseConfig() {
		os.Exit(1)
	}
	initDB()
	defer db.Close()

	http.HandleFunc("/ip", ipHandler)
	http.HandleFunc("/notify", authWrapper(notifyHandler))
	http.HandleFunc("/puturl", authWrapper(saveHandler))
	http.HandleFunc("/geturl", retrieveHandler)

	if cfg.Section("general").Key("httpenabled").String() == "true" {
		go func() {
			err := http.ListenAndServe(
				":"+cfg.Section("general").Key("httpport").String(),
				nil,
			)
			if err != nil {
				fmt.Printf("could not start http server: %v\n", err)
				os.Exit(1)
			}
		}()
	}

	if cfg.Section("general").Key("sslenabled").String() == "true" {
		go func() {
			err := http.ListenAndServeTLS(
				":"+cfg.Section("general").Key("sslport").String(),
				cfg.Section("general").Key("sslcert").String(),
				cfg.Section("general").Key("sslkey").String(),
				nil,
			)
			if err != nil {
				fmt.Printf("could not start ssl server: %s\n", err)
				os.Exit(1)
			}
		}()
	}

	// todo: add proper shutdown logic
	select {}
}
