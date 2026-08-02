package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"api/internal/llm"
	"api/internal/mlflow"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type PromptRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

type PromptResponse struct {
	Reply string `json:"reply"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadSystemPrompt resolves the system prompt at request time.
// If MLFLOW_PROMPT_URI is set (e.g. "prompts:/assistant/production"), the
// template is fetched from the MLflow Prompt Registry. Otherwise it falls back
// to the SYSTEM_PROMPT env var (default: "You are a helpful assistant.").
func loadSystemPrompt(registry *mlflow.RegistryClient) string {
	if uri := getEnv("MLFLOW_PROMPT_URI", ""); uri != "" {
		template, err := registry.LoadPrompt(uri)
		if err != nil {
			log.Printf("prompt registry: could not load %q, falling back to SYSTEM_PROMPT: %v", uri, err)
		} else {
			return template
		}
	}
	return getEnv("SYSTEM_PROMPT", "You are a helpful assistant.")
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

// newRouter wires up all routes and middleware. It is extracted from main so
// that integration tests can create a testable *gin.Engine without starting a
// real listener.
func newRouter(
	db *sql.DB,
	llmClient *llm.Client,
	mlflowTracer *mlflow.Tracer,
	promptRegistry *mlflow.RegistryClient,
	sessionSecret string,
	frontendOrigin string,
) *gin.Engine {
	r := gin.Default()

	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   3600 * 24,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("session", store))

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{frontendOrigin},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
	}))

	r.POST("/login", func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		var hashedPassword string
		err := db.QueryRow("SELECT password_hash FROM users WHERE username = $1", req.Username).Scan(&hashedPassword)
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		session := sessions.Default(c)
		session.Set("user", req.Username)
		session.Save()
		c.JSON(http.StatusOK, gin.H{"username": req.Username})
	})

	r.POST("/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		session.Save()
		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
	})

	r.GET("/me", func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"username": user})
	})

	protected := r.Group("/")
	protected.Use(authMiddleware())
	{
		protected.POST("/prompt", func(c *gin.Context) {
			var req PromptRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			user, _ := c.Get("user")
			username, _ := user.(string)

			systemPrompt := loadSystemPrompt(promptRegistry)
			startTime := time.Now()

			llmResp, err := llmClient.Chat(c.Request.Context(), systemPrompt, req.Prompt)
			duration := time.Since(startTime)

			if err != nil {
				log.Printf("llm error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get LLM response"})
				return
			}

			go mlflowTracer.LogLLMTrace(
				username, req.Prompt, llmResp.Content, llmResp.Model,
				llmResp.InputTokens, llmResp.OutputTokens,
				startTime, duration,
			)

			c.JSON(http.StatusOK, PromptResponse{Reply: llmResp.Content})
		})
	}

	return r
}

func main() {
	// Load .env if present (ignored in production where env vars are set externally)
	_ = godotenv.Load()

	dsn := getEnv("DATABASE_URL", "")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	sessionSecret := getEnv("SESSION_SECRET", "")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET environment variable is required")
	}

	llmClient := llm.NewClient()
	mlflowTracer := mlflow.NewTracer()
	promptRegistry := mlflow.NewRegistryClient()
	frontendOrigin := getEnv("FRONTEND_ORIGIN", "http://localhost:5173")

	r := newRouter(db, llmClient, mlflowTracer, promptRegistry, sessionSecret, frontendOrigin)
	r.Run(":8080")
}
