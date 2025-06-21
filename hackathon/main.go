package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"  
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
	"github.com/gorilla/mux"
)

type UserResForHTTPGet struct {
	ID        string `json:"id"`     
    Username    string `json:"username"` 
    Content   string `json:"content"`
    CreatedAt time.Time `json:"created_at"`
	LikesCount   int       `json:"likes_count"` 
	ReplyCount   int	   `json:"reply_count"`
    IsLikedByMe  bool      `json:"is_liked_by_me"`
	ProfileImageUrl sql.NullString `json:"profile_image_url"`
	ImageUrl sql.NullString `json:"image_url"`
}
type LikeRequest struct {
    PostID string `json:"post_id"`
}
type UserProfile struct {
	Username    string `json:"username"`
	ProfileImageUrl sql.NullString `json:"profile_image_url"`
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
    connStr := fmt.Sprintf("%s:%s@%s/%s?parseTime=True", mysqlUser, mysqlPwd, mysqlHost, mysqlDatabase)
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
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT")
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
			ProfileImageUrl string `json:"profileImageUrl"`
		}
		err = json.NewDecoder(r.Body).Decode(&reqBody)
		log.Printf("Received request body: %+v\n", reqBody)
		log.Printf("Received profileUrl: %s\n", reqBody.ProfileImageUrl)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: decode to json, %v\n", err)
			return
		}
		username := reqBody.Username
		url := reqBody.ProfileImageUrl
		var sqlurl sql.NullString
        if url != "" {
            sqlurl = sql.NullString{String: url, Valid: true}
        } else {
            sqlurl = sql.NullString{Valid: false}
        }
		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: db begin, %v\n", err)
			return
		}
		stmt, err := db.Prepare("INSERT INTO users(id, username, email, profile_image_url ) VALUES(?, ?, ?, ?)")
		if err != nil {
			tx.Rollback()
			log.Printf("insert into sql, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		_, err = stmt.Exec(id, username, email, sqlurl)
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
		authHeader := r.Header.Get("Authorization")
    	idToken := strings.TrimPrefix(authHeader, "Bearer ")
    	ctx := r.Context()
    	client, err := firebaseApp.Auth(ctx)
    	if err != nil {
        	log.Printf("fail: get firebase auth client, %v\n", err)
        	w.WriteHeader(http.StatusInternalServerError)
        	return
    	}
    	token, err := client.VerifyIDToken(ctx, idToken)
    	if err != nil {
        	log.Printf("fail: verify ID token, %v\n", err)
        	w.WriteHeader(http.StatusUnauthorized)
        	return
    	}
    	id := token.UID 
        query := `
        SELECT 
            u.Username, 
            u.profile_image_url
        FROM 
            users u
        WHERE
            u.id = ?`
        row := db.QueryRow(query, id)
        var p UserProfile
        row.Scan(&p.Username,&p.ProfileImageUrl);
        w.Header().Set("Content-Type", "application/json")
        if err := json.NewEncoder(w).Encode(p); err != nil {
            log.Printf("エラー: JSONエンコードに失敗しました, %v\n", err)
            return
        }
	case http.MethodPut:
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
			Username string `json:"username"`
			ProfileImageUrl string `json:"profileImageUrl"`
		}
		err = json.NewDecoder(r.Body).Decode(&reqBody)
		log.Printf("Received request body: %+v\n", reqBody)
		log.Printf("Received profileUrl: %s\n", reqBody.ProfileImageUrl)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: decode to json, %v\n", err)
			return
		}
		username := reqBody.Username
		url := reqBody.ProfileImageUrl
		var sqlurl sql.NullString
        if url != "" {
            sqlurl = sql.NullString{String: url, Valid: true}
        } else {
            sqlurl = sql.NullString{Valid: false}
        }
		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: db begin, %v\n", err)
			return
		}
		stmt, err := db.Prepare("UPDATE users SET username = ?,profile_image_url = ? WHERE id = ?;" )
		if err != nil {
			tx.Rollback()
			log.Printf("insert into sql, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		_, err = stmt.Exec(username, sqlurl, id)
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
			PostID string `json:"reply_id"`
			Content string `json:"content"`
			ImageUrl string `json:"imageUrl"`
		}
		err = json.NewDecoder(r.Body).Decode(&reqBody)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: decode to json, %v\n", err)
			return
		}
		content := reqBody.Content
		reply := reqBody.PostID
		image := reqBody.ImageUrl
		newPostID := generateUUID()
		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("fail: db begin, %v\n", err)
			return
		}
		stmt, err := db.Prepare("INSERT INTO posts(id, user_id, content_text,reply_to_post_id, image_url) VALUES(?, ?, ?, ?, ?)")
		if err != nil {
			tx.Rollback()
			log.Printf("insert into sql, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		var sqlReply sql.NullString
        if reply != "" {
            sqlReply = sql.NullString{String: reply, Valid: true}
        } else {
            sqlReply = sql.NullString{Valid: false}
        }
		var sqlImage sql.NullString
        if image!= "" {
            sqlImage = sql.NullString{String: image, Valid: true}
        } else {
            sqlImage = sql.NullString{Valid: false}
        }

		_, err = stmt.Exec(newPostID,id,content,sqlReply,sqlImage)
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
		searchQuery := r.URL.Query().Get("q")
    	baseQuery := `
        	SELECT 
        	    p.id, 
        	    u.Username, 
        	    p.content_text, 
        	    p.created_at, 
        	    COUNT(l.id) AS likes_count,
        	    (SELECT COUNT(*) FROM posts AS r WHERE r.reply_to_post_id = p.id) AS reply_count,
        	    CASE WHEN EXISTS (SELECT 1 FROM likes WHERE post_id = p.id AND user_id = ?) THEN TRUE ELSE FALSE END AS is_liked_by_me,
        	    u.profile_image_url,
        	    p.image_url
        	FROM posts p
        	LEFT JOIN users u ON p.user_id = u.id
        	LEFT JOIN likes l ON p.id = l.post_id
    	`
    	whereClause := " WHERE 1=1 "
    	args := []interface{}{id}
    	if searchQuery != "" {
    	    whereClause += " AND (p.content_text LIKE ? OR u.Username LIKE ?) "
    	    searchPattern := "%" + searchQuery + "%"
    	    args = append(args, searchPattern, searchPattern)
    	}
   		groupByClause := `
        	GROUP BY p.id, u.Username, p.content_text, p.created_at, u.profile_image_url, p.image_url
        	ORDER BY p.created_at DESC
    	`
    
    	finalQuery := baseQuery + whereClause + groupByClause

    	rows, err := db.Query(finalQuery,args... )
		if err != nil {
			log.Printf("fail: db.Query, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		posts := make([]UserResForHTTPGet, 0)
		for rows.Next() {
			var u UserResForHTTPGet
			if err := rows.Scan(&u.ID, &u.Username, &u.Content, &u.CreatedAt, &u.LikesCount, &u.ReplyCount, &u.IsLikedByMe, &u.ProfileImageUrl, &u.ImageUrl); err != nil {
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

func likehandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
    w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE, OPTIONS") 
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusOK)
        return
    }

    authHeader := r.Header.Get("Authorization")
    idToken := strings.TrimPrefix(authHeader, "Bearer ")
    ctx := r.Context()
    client, err := firebaseApp.Auth(ctx)
    if err != nil {
        log.Printf("fail: get firebase auth client, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
        return
    }
    token, err := client.VerifyIDToken(ctx, idToken)
    if err != nil {
        log.Printf("fail: verify ID token, %v\n", err)
		w.WriteHeader(http.StatusUnauthorized)
        return
    }
    likingUserID := token.UID 

    var reqBody LikeRequest
    if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
        log.Printf("fail: decode request body, %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
        return
    }
    postID := reqBody.PostID

    if postID == "" {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("fail: no postID, %v\n", err)
        return
    }

    tx, err := db.Begin()
    if err != nil {
        log.Printf("fail: db.Begin, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
        return
    }
    defer tx.Rollback() 

    switch r.Method {
    case http.MethodPost:
        likeID := generateUUID() 
        stmt, err := tx.Prepare("INSERT INTO likes (id, post_id, user_id) VALUES (?, ?, ?)")
        if err != nil {
            log.Printf("fail: tx.Prepare INSERT, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
            return
        }
        defer stmt.Close()

        _, err = stmt.Exec(likeID, postID, likingUserID)
        if err != nil {
            if strings.Contains(err.Error(), "Duplicate entry") { 
                log.Printf("info: user %s already liked post %s\n", likingUserID, postID)
				w.WriteHeader(http.StatusConflict)
                return
            }
            log.Printf("fail: stmt.Exec INSERT, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
            return
        }
        if err = tx.Commit(); err != nil {
            log.Printf("fail: tx.Commit INSERT, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusCreated) 
        log.Printf("User %s liked post %s", likingUserID, postID)

    case http.MethodGet:
        stmt, err := tx.Prepare("DELETE FROM likes WHERE post_id = ? AND user_id = ?")
        if err != nil {
            log.Printf("fail: tx.Prepare DELETE, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			tx.Rollback()
            return
        }
        defer stmt.Close()
        result, err := stmt.Exec(postID, likingUserID)
        if err != nil {
            log.Printf("fail: stmt.Exec DELETE, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			tx.Rollback()
            return
        }
		rowsAffected, err := result.RowsAffected()
		if err != nil {

        	log.Printf("fail: RowsAffected DELETE, %v\n", err)
        	w.WriteHeader(http.StatusInternalServerError) 
        	tx.Rollback()
        	return
    	}

    	if rowsAffected == 0 {
        	log.Printf("no matching record found.", likingUserID, postID)
        	w.WriteHeader(http.StatusNotFound) 
        	tx.Rollback() 
        	return
    	}
		if err := tx.Commit(); err != nil { 
        	log.Printf("fail: tx.Commit DELETE, %v\n", err)
        	w.WriteHeader(http.StatusInternalServerError)
        	return
		}
        w.WriteHeader(http.StatusOK) 

    default:
		w.WriteHeader(http.StatusMethodNotAllowed)
    }
}



func replieshandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
    w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS") 
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusOK)
        return
    }

    authHeader := r.Header.Get("Authorization")
    idToken := strings.TrimPrefix(authHeader, "Bearer ")
    ctx := r.Context()
    client, err := firebaseApp.Auth(ctx)
    if err != nil {
        log.Printf("fail: get firebase auth client, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
        return
    }
    token, err := client.VerifyIDToken(ctx, idToken)
    if err != nil {
        log.Printf("fail: verify ID token, %v\n", err)
		w.WriteHeader(http.StatusUnauthorized)
        return
    }
    id := token.UID 
	vars := mux.Vars(r)
	postID := vars["postId"]
    switch r.Method {
    case http.MethodGet:
		query := `
		SELECT 
            p.id, 
            u.Username, 
			p.content_text, 
			p.created_at,
            COUNT(l.id) AS likes_count,
			(SELECT COUNT(*) FROM posts AS r WHERE r.reply_to_post_id = p.id) AS reply_count,
            CASE WHEN EXISTS (SELECT 1 FROM likes WHERE post_id = p.id AND user_id = ?) THEN TRUE ELSE FALSE END AS is_liked_by_me,
			u.profile_image_url,
			p.image_url
        FROM 
            posts p
		LEFT JOIN
			users u ON p.user_id = u.id 
        LEFT JOIN 
            likes l ON p.id = l.post_id
		WHERE
			p.reply_to_post_id = ?
		GROUP BY
            p.id, u.Username, p.content_text, p.created_at, p.reply_to_post_id
		ORDER BY 
            p.created_at ASC`
		rows, err := db.Query(query, id, postID)
		if err != nil {
			log.Printf("fail: db.Query, %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		posts := make([]UserResForHTTPGet, 0)
		for rows.Next() {
			var u UserResForHTTPGet
			if err := rows.Scan(&u.ID, &u.Username, &u.Content, &u.CreatedAt, &u.LikesCount, &u.ReplyCount, &u.IsLikedByMe, &u.ProfileImageUrl, &u.ImageUrl); err != nil {
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
		w.WriteHeader(http.StatusMethodNotAllowed)
    }
}
func detailhandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
    w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS") 
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusOK)
        return
    }

    authHeader := r.Header.Get("Authorization")
    idToken := strings.TrimPrefix(authHeader, "Bearer ")
    ctx := r.Context()
    client, err := firebaseApp.Auth(ctx)
    if err != nil {
        log.Printf("fail: get firebase auth client, %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
        return
    }
    token, err := client.VerifyIDToken(ctx, idToken)
    if err != nil {
        log.Printf("fail: verify ID token, %v\n", err)
		w.WriteHeader(http.StatusUnauthorized)
        return
    }
    id := token.UID 
	vars := mux.Vars(r)
	postID := vars["postId"]
	log.Printf("postID:, %v\n", postID)
    switch r.Method {
    case http.MethodGet:
		query := `
		SELECT 
            p.id, 
            u.Username, 
			p.content_text,
			p.created_at,
            COUNT(l.id) AS likes_count,
			(SELECT COUNT(*) FROM posts AS r WHERE r.reply_to_post_id = p.id) AS reply_count,
            CASE WHEN EXISTS (SELECT 1 FROM likes WHERE post_id = p.id AND user_id = ?) THEN TRUE ELSE FALSE END AS is_liked_by_me,
			u.profile_image_url,
			p.image_url
        FROM 
            posts p
		LEFT JOIN
			users u ON p.user_id = u.id 
        LEFT JOIN 
            likes l ON p.id = l.post_id
		WHERE
			p.id = ?
		GROUP BY
            p.id, u.Username, p.content_text, p.created_at
		ORDER BY 
            p.created_at DESC`
		row := db.QueryRow(query, id, postID)
		var p UserResForHTTPGet
		row.Scan(&p.ID, &p.Username, &p.Content, &p.CreatedAt, &p.LikesCount, &p.ReplyCount, &p.IsLikedByMe, &p.ProfileImageUrl, &p.ImageUrl);
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(p); err != nil {
			log.Printf("エラー: JSONエンコードに失敗しました, %v\n", err)
			return
		}
    default:
		w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/user", userhandler).Methods("POST","GET","PUT", "OPTIONS")
    router.HandleFunc("/post_detail/{postId}", detailhandler).Methods("GET", "OPTIONS")
    router.HandleFunc("/replies/{postId}", replieshandler).Methods("GET", "OPTIONS")
    router.HandleFunc("/post", posthandler).Methods("GET", "POST", "OPTIONS")
    router.HandleFunc("/likes", likehandler).Methods("POST", "DELETE", "OPTIONS")
	// ③ Ctrl+CでHTTPサーバー停止時にDBをクローズする
	closeDBWithSysCall()

	// 8080番ポートでリクエストを待ち受ける
	log.Println("Listening...")
	if err := http.ListenAndServe(":8080", router); err != nil {
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
// git push用　メモ