package main

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type PromptRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

type PromptResponse struct {
	Reply string `json:"reply"`
}

func main() {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: false,
	}))

	r.OPTIONS("/prompt", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	r.POST("/prompt", func(c *gin.Context) {
		var req PromptRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, PromptResponse{Reply: req.Prompt})
	})

	r.Run(":8080")
}
