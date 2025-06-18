package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"context"
    "log"
    "strings" 
	"github.com/google/uuid"
    "firebase.google.com/go"
    "firebase.google.com/go/auth" 
    "google.golang.org/api/option"
)

type UserResForHTTPGet struct {
	ID        string `json:"id"`         // JSONキーを "id" にする
    UserID    string `json:"user_id"`    // JSONキーを "user_id" にする
    Content   string `json:"content"`
    CreatedAt time.Time `json:"created_at"` // JSONキーを "created_at" にする
}

var db *sql.DB
var firebaseApp *firebase.App
func init() {
	secretkey := os.Getenv("FIREBASE_SECRET_KEY")
	opt := option.WithCredentialsJSON([]byte(secretkey))
    app, err := firebase.NewApp(context.Background(), nil, opt)
    if err != nil {
        log.Fatalf("Firebase app initialization error: %v\n", err)
    }
    firebaseApp = app
    log.Printf("Firebase Admin SDK initialized")
    mysqlUser := os.Getenv("MYSQL_USER")
    mysqlPwd := os.Getenv("MYSQL_PWD")
    mysqlHost := os.Getenv("MYSQL_HOST")
    mysqlDatabase := os.Getenv("MYSQL_DATABASE")
    connStr := fmt.Sprintf("%s:%s@%s/%s", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
    db, err = sql.Open("mysql", connStr)
	if err != nil {
		log.Fatalf("fail: sql.Open, %v\n", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("fail: _db.Ping, %v\n", err)
	}
	log.Println("Successfully connected to the database")

	_ = auth.Client{} //エラー回避用
}
func generateUUID() string {
	return uuid.New().String() 
}

func userhandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
		authHeader := r.Header.Get("Authorization")
		idToken := strings.TrimPrefix(authHeader, "Bearer ")
		ctx := r.Context() 
    	client, err := firebaseApp.Auth(ctx)
    	if err != nil {
        	w.WriteHeader(http.StatusInternalServerError)
        	log.Printf("fail: get firebase auth client, %v\n", err)
        	return
    	}
		token, err := client.VerifyIDToken(ctx, idToken)
    	if err != nil {
        w.WriteHeader(http.StatusUnauthorized)
        log.Printf("fail: verify ID token, %v\n", err)
        return
    	}
		id := token.UID
		email := "" 
		if emailClaim, ok := token.Claims["email"]; ok {
    		if emailStr, isString := emailClaim.(string); isString {
        		email = emailStr
    		}
		}
		var reqBody struct {
			Username string `json:"username"`
		}
		err = json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: decode to json, %v\n", err)
			return
		}
		username := reqBody.Username
		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: db begin, %v\n", err)
			return
		}
		stmt, err := db.Prepare("INSERT INTO users(id, username, email) VALUES(?, ?, ?)")
		if err != nil {
			tx.Rollback()
			log.Printf("insert into sql, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		_, err = stmt.Exec(id, username, email)
		if err != nil {
			tx.Rollback()
			log.Printf("fail:stmt, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err = tx.Commit(); err != nil {
			tx.Rollback()
			log.Printf("fail: commit, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		log.Printf("fail: HTTP Method is %s\n", r.Method)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func posthandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
		authHeader := r.Header.Get("Authorization")
		idToken := strings.TrimPrefix(authHeader, "Bearer ")
		ctx := r.Context() 
    	client, err := firebaseApp.Auth(ctx)
    	if err != nil {
        	w.WriteHeader(http.StatusInternalServerError)
        	log.Printf("fail: get firebase auth client, %v\n", err)
        	return
    	}
		token, err := client.VerifyIDToken(ctx, idToken)
    	if err != nil {
        w.WriteHeader(http.StatusUnauthorized)
        log.Printf("fail: verify ID token, %v\n", err)
        return
    	}
		id := token.UID
		var reqBody struct {
			Content string `json:"content"`
		}
		err = json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: decode to json, %v\n", err)
			return
		}
		content := reqBody.Content
		newPostID := generateUUID()
		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: db begin, %v\n", err)
			return
		}
		stmt, err := db.Prepare("INSERT INTO posts(id, user_id, content_text) VALUES(?, ?, ?)")
		if err != nil {
			tx.Rollback()
			log.Printf("insert into sql, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		_, err = stmt.Exec(newPostID,id,content)
		if err != nil {
			tx.Rollback()
			log.Printf("fail:stmt, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err = tx.Commit(); err != nil {
			tx.Rollback()
			log.Printf("fail: commit, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		rows, err := db.Query("SELECT id, user_id, content, created_at FROM posts ORDER BY created_at DESC")
		if err != nil {
			log.Printf("fail: db.Query, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		posts := make([]UserResForHTTPGet, 0)
		for rows.Next() {
			var u UserResForHTTPGet
			if err := rows.Scan(&u.ID, &UserID, &u.Content, &u.CreatedAt); err != nil {
				log.Printf("fail: rows.Scan, %v\n", err)
				if err := rows.Close(); err != nil {
					log.Printf("fail: rows.Close(), %v\n", err)
				}
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			posts = append(posts, u)
		}
		bytes, err := json.Marshal(posts)
		if err != nil {
			log.Printf("fail: json.Marshal, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(bytes)
	default:
		log.Printf("fail: HTTP Method is %s\n", r.Method)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func main() {
	http.HandleFunc("/user", userhandler)
	http.HandleFunc("/post", posthandler)

	// ③ Ctrl+CでHTTPサーバー停止時にDBをクローズする
	closeDBWithSysCall()

	// 8080番ポートでリクエストを待ち受ける
	log.Println("Listening...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// ③ Ctrl+CでHTTPサーバー停止時にDBをクローズする
func closeDBWithSysCall() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sig
		log.Printf("received syscall, %v", s)

		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
		log.Printf("success: db.Close()")
		os.Exit(0)
	}()
}
// git push用　メモメモ