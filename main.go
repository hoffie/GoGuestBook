package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// Entry represents a single entry in the guestbook
type Entry struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Email     string    `json:"email" db:"email"`
	Message   string    `json:"message" db:"message"`
	Approved  int8      `json:"approved" db:"approved"`
	Comment   string    `json:"comment" db:"comment"`
	IP        string    `json:"-" db:"ip"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Code      string    `json:"code" db:"-"`
}

// Configuration holds the environment variables
type Configuration struct {
	DBFile            string
	Listen            string
	SMTPHost          string
	SMTPPort          string
	SMTPUser          string
	SMTPPass          string
	AdminEmail        string
	URL               string
	EntryWaitDuration time.Duration
	AntiSpamCode      string
}

const (
	mailEntryApprovedSubject   = "Gästebuch-Eintrag freigeschaltet"
	mailEntryApprovedBody      = "Dein Gästebuch-Eintrag wurde freigeschaltet: %s"
	mailEntryCommentSubject    = "Gästebuch-Eintrag kommentiert"
	mailEntryCommentBody       = "Dein Gästebuch-Eintrag wurde kommentiert: %s"
	mailAdminEntryAddedSubject = "Neuer Gästebuch-Eintrag"
	mailAdminEntryAddedBody    = "Neuer Gästebucheintrag, bitte prüfen und freischalten: %s"
	nameMinLen                 = 3
	nameMaxLen                 = 100
	emailMinLen                = 6
	emailMaxLen                = 100
	messageMinLen              = 10
	messageMaxLen              = 2000
)

var (
	db     *sqlx.DB
	config Configuration
)

func main() {
	// Load environment variables
	loadConfig()

	// Connect to SQLite database
	var err error
	db, err = sqlx.Connect("sqlite3", config.DBFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create table if it doesn't exist
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS entries (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT NOT NULL,
            message TEXT NOT NULL,
			ip TEXT NOT NULL,
            approved INTEGER NOT NULL DEFAULT 0,
            comment TEXT NOT NULL,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
    `)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize router
	router := mux.NewRouter()

	// Define routes
	router.HandleFunc("/api/entries", getApprovedEntries).Methods("GET")
	router.HandleFunc("/api/entries", createEntry).Methods("POST")
	router.HandleFunc("/api/entries/{id}", getEntry).Methods("GET")
	router.HandleFunc("/api/entries/{id}/approve", approveEntry).Methods("POST")
	router.HandleFunc("/api/entries/{id}/reject", rejectEntry).Methods("POST")
	router.HandleFunc("/api/entries/{id}/comment", addComment).Methods("PUT")
	router.HandleFunc("/static/demo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		http.ServeFile(w, r, "demo.html")
	})
	router.HandleFunc("/static/GoGuestBook.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		http.ServeFile(w, r, "GoGuestBook.js")
	})

	listen := config.Listen
	log.Println(fmt.Sprintf("Server listening on %s", listen))
	log.Fatal(http.ListenAndServe(listen, router))
}

// loadConfig loads environment variables into the configuration struct
func loadConfig() {
	config.DBFile = getEnv("DB_FILE")
	config.Listen = getEnv("LISTEN")
	config.SMTPHost = getEnv("SMTP_HOST")
	config.SMTPPort = getEnv("SMTP_PORT")
	config.SMTPUser = getEnv("SMTP_USER")
	config.SMTPPass = getEnv("SMTP_PASS")
	config.AdminEmail = getEnv("ADMIN_EMAIL")
	config.AntiSpamCode = getEnv("ANTI_SPAM_CODE")
	config.URL = getEnv("URL")
	duration, err := time.ParseDuration(fmt.Sprintf("%ss", getEnv("ENTRY_WAIT_SECONDS")))
	if err != nil {
		log.Fatal("Failed to parse ENTRY_WAIT_SECONDS")
	}
	config.EntryWaitDuration = duration
}

func getEnv(key string) string {
	key = fmt.Sprintf("GGB_%s", key)
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	log.Fatal("Failed to get env variable ", key)
	return ""
}

// sendEmail sends an email notification
func sendEmail(to string, subject string, body string) error {
	smtpHost := config.SMTPHost
	smtpPort := config.SMTPPort
	smtpUser := config.SMTPUser
	smtpPass := config.SMTPPass
	from := config.AdminEmail

	// Prepare the email message
	message := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: =?utf-8?q?" + subject + "?=\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		body)

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{to}, message)
	return err
}

// getApprovedEntries returns all approved guestbook entries
func getApprovedEntries(w http.ResponseWriter, r *http.Request) {
	var entries []Entry
	// never leak ID or IP here!
	err := db.Select(&entries, "SELECT name, message, approved, comment, created_at FROM entries WHERE approved = 1 ORDER by created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// createEntry creates a new guestbook entry
func createEntry(w http.ResponseWriter, r *http.Request) {
	var entry Entry
	err := json.NewDecoder(r.Body).Decode(&entry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Simplistic spam protection
	if entry.Code != config.AntiSpamCode {
		http.Error(w, "", http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "code"})
		return
	}

	// Input validation
	if err := validateEntry(&entry); err != nil {
		http.Error(w, "", http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "validation"})
		return
	}

	// Enforce wait time from same IP between new posts
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	var prevEntry Entry
	err = db.Get(&prevEntry, "SELECT created_at FROM entries WHERE ip = ? ORDER BY created_at DESC LIMIT 1", ip)
	if err == nil {
		if time.Since(prevEntry.CreatedAt) < config.EntryWaitDuration {
			http.Error(w, "", http.StatusTooManyRequests)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "postlimit"})
			return
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save the entry
	id, err := generateID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry.ID = id
	entry.IP = ip
	_, err = db.NamedExec("INSERT INTO entries (id, name, email, message, ip, comment) VALUES (:id, :name, :email, :message, :ip, '')", entry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send email notification to admin
	url := config.URL
	url = fmt.Sprintf("%s?GgbEntryID=%s", url, id)
	log.Printf("New entry: %s", url)
	err = sendEmail(config.AdminEmail, mailAdminEntryAddedSubject, fmt.Sprintf(mailAdminEntryAddedBody, url))
	if err != nil {
		log.Printf("Failed to send email to admin: %v", err)
	}
	w.WriteHeader(http.StatusCreated)
	// Do not leak the random ID to the submitter here!
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// validateEntry checks the validity of the guestbook entry fields
func validateEntry(entry *Entry) error {
	if len(entry.Name) < nameMinLen || len(entry.Name) > nameMaxLen {
		return fmt.Errorf("name must be between %d and %d characters", nameMinLen, nameMaxLen)
	}
	if len(entry.Email) < emailMinLen || len(entry.Email) > emailMaxLen {
		return fmt.Errorf("email must be between %d and %d characters", emailMinLen, emailMaxLen)
	}
	if len(entry.Message) < messageMinLen || len(entry.Message) > messageMaxLen {
		return fmt.Errorf("message must be between %d and %d characters", messageMinLen, messageMaxLen)
	}

	// Simple email format validation (basic regex)
	if !isValidEmail(entry.Email) {
		return errors.New("invalid email format")
	}

	return nil
}

// isValidEmail checks if the email format is valid
func isValidEmail(email string) bool {
	// A simple regex for validating an email address
	const emailRegex = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	re := regexp.MustCompile(emailRegex)
	return re.MatchString(email)
}

// generateID generates a cryptographically secure random string as ID
func generateID() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}

// getEntry returns a single guestbook entry by ID
func getEntry(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]
	var entry Entry
	err := db.Get(&entry, "SELECT * FROM entries WHERE id = ?", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// approveEntry approves a guestbook entry by ID
func approveEntry(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]

	// Retrieve the previous approval status and author's email
	var oldEntry Entry
	err := db.Get(&oldEntry, "SELECT email, approved FROM entries WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := db.Exec("UPDATE entries SET approved = 1 WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	// Send email to the author
	if oldEntry.Email != "" && oldEntry.Approved == 0 {
		// Don't attempt to send to empty mail addresses.
		// Don't send mail multiple times for the same post.
		err = sendEmail(oldEntry.Email, mailEntryApprovedSubject, fmt.Sprintf(mailEntryApprovedBody, config.URL))
		if err != nil {
			log.Printf("Failed to send email to author: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// rejectEntry rejects a guestbook entry by ID
func rejectEntry(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]
	result, err := db.Exec("UPDATE entries SET approved = -1 WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addComment adds a comment to a guestbook entry by ID
func addComment(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]

	// Retrieve the author's email and previous comment state
	var oldEntry Entry
	err := db.Get(&oldEntry, "SELECT email, comment FROM entries WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var comment struct {
		Comment string `json:"comment"`
	}
	err = json.NewDecoder(r.Body).Decode(&comment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update the entry with the new comment
	result, err := db.Exec("UPDATE entries SET comment = ? WHERE id = ?", comment.Comment, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	// Send email to the author
	if oldEntry.Email != "" && oldEntry.Comment == "" {
		// Don't send mail if no email address is stored.
		// Don't send mail multiple times for the same post.
		err = sendEmail(oldEntry.Email, mailEntryCommentSubject, fmt.Sprintf(mailEntryCommentBody, config.URL))
		if err != nil {
			log.Printf("Failed to send email to author: %v", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
