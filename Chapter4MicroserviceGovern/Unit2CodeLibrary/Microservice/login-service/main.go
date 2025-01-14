package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/rs/cors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Success   bool   `json:"success"`
	AuthToken string `json:"authToken"`
	ID        int    `json:"id"` // 使用 'ID' 而不是 'UserID'
}

func main() {
	initNacos() // Initialize Nacos client
	initDatabase()
	defer closeDatabase()

	err := registerService(NamingClient, "login-service", "127.0.0.1", 8083)
	if err != nil {
		fmt.Printf("Error registering game service instance: %v\n", err)
		os.Exit(1)
	}
	defer closeDatabase()
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://micro.roliyal.com"}, // 明确指定前端地址
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},    // 包含 OPTIONS 方法
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-User-ID"}, // 明确列出允许的请求头
		Debug:            true,                                                   // 启用调试日志

	})

	mux := http.NewServeMux()
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/user", userHandler)
	mux.HandleFunc("/register", registerHandler)

	// 应用 CORS 中间件到整个 ServeMux
	handler := c.Handler(mux)

	fmt.Println("Starting server on port 8083")
	log.Fatal(http.ListenAndServe(":8083", handler))

}
func updateUser(user *User) error {
	if err := db.Model(user).Where("id = ?", user.ID).Update("auth_token", user.AuthToken).Error; err != nil {
		log.Println("Error updating user:", err)
		return err
	}
	return nil
}

func generateAuthToken() (string, error) {
	return generateRandomToken(32)
}

func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received login request")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error reading request body:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req loginRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Println("Error unmarshalling JSON:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Received login request with username: %s, password: %s\n", req.Username, req.Password)

	var user User
	db = db.LogMode(true)

	if err := db.Select("ID, Username, Password, Wins, Attempts, AuthToken").Where("username = ? AND password = ?", req.Username, req.Password).First(&user).Error; err == nil {
		log.Println("User found:", user)

		// 生成新的 AuthToken
		newAuthToken, err := generateAuthToken()
		if err != nil {
			log.Println("Error generating auth token:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 更新用户的 AuthToken
		user.AuthToken = newAuthToken
		if err := db.Save(&user).Error; err != nil {
			log.Println("Error updating user with new AuthToken:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		res := loginResponse{
			Success:   true,
			AuthToken: newAuthToken,
			ID:        user.ID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		fmt.Println("Sent login response:", res)
	} else {
		log.Println("User not found, error:", err)
		res := loginResponse{
			Success: false,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
		fmt.Println("Sent login response:", res)
	}
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	authToken := r.Header.Get("Authorization")
	userID := r.Header.Get("X-User-ID") // 获取 X-User-ID 请求头

	if authToken == "" || userID == "" {
		log.Println("Error: Missing Authorization or X-User-ID header")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Missing Authorization or X-User-ID header",
		})
		return
	}

	// 将 userID 转换为整数
	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		log.Println("Error parsing userID:", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Invalid userID",
		})
		return
	}

	// 使用 authToken 和 userID 查询用户
	var user User
	if err := db.Where("auth_token = ? AND id = ?", authToken, userIDInt).First(&user).Error; err != nil {
		log.Printf("Error finding user by authToken and userID: %v\n", err)
		if gorm.IsRecordNotFoundError(err) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// 返回用户数据
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received register request")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error reading request body:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req registerRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Println("Error unmarshalling JSON:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Received register request with username: %s, password: %s\n", req.Username, req.Password)

	var user User
	err = db.Where("username = ?", req.Username).First(&user).Error
	if err == nil {
		log.Println("Username already exists:", req.Username)
		w.WriteHeader(http.StatusConflict)
		return
	}

	if !gorm.IsRecordNotFoundError(err) {
		log.Println("Error checking for existing user:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	newAuthToken, err := generateAuthToken()
	if err != nil {
		log.Println("Error generating auth token:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user = User{
		Username:  req.Username,
		Password:  req.Password,
		AuthToken: newAuthToken,
		Wins:      0,
		Attempts:  0,
	}

	err = db.Create(&user).Error
	if err != nil {
		log.Println("Error creating new user:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res := loginResponse{
		Success:   true,
		AuthToken: newAuthToken,
		ID:        user.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res)
}
