package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"context"
    "log"
    "strings" 
    firebase "firebase.google.com/go"
    "firebase.google.com/go/auth" 
    "google.golang.org/api/option"
)

type UserResForHTTPGet struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var db *sql.DB
var firebaseApp *firebase.App
func init() {
	secretkey := os.Getenv("FIERBASE_SECRET_KEY")
	opt := option.WithCredentialsJSON([]byte(secretkey))
	log.Printf("FIERBASE_SECRET_KEY length: %d bytes\n", len(secretkey))
    if len(secretkey) > 50 { // キーが長すぎるので先頭と末尾だけ表示
        log.Printf("FIERBASE_SECRET_KEY start: %s..., end: ...%s\n", secretkey[:20], secretkey[len(secretkey)-20:])
    } else {
        log.Printf("FIERBASE_SECRET_KEY: %s\n", secretkey)
    }
    app, err := firebase.NewApp(context.Background(), nil, opt)
    if err != nil {
        log.Fatalf("Firebase app initialization error: %v\n", err)
    }
    firebaseApp = app
    log.Println("Firebase Admin SDK initialized successfully.")
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

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodGet:
		rows, err := db.Query("SELECT name, age FROM users")
		if err != nil {
			log.Printf("fail: db.Query, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		users := make([]UserResForHTTPGet, 0)
		for rows.Next() {
			var u UserResForHTTPGet
			if err := rows.Scan(&u.Name, &u.Age); err != nil {
				log.Printf("fail: rows.Scan, %v\n", err)
				if err := rows.Close(); err != nil {
					log.Printf("fail: rows.Close(), %v\n", err)
				}
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			users = append(users, u)
		}
		bytes, err := json.Marshal(users)
		if err != nil {
			log.Printf("fail: json.Marshal, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(bytes)
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

func main() {
	http.HandleFunc("/user", handler)

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