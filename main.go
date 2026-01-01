package main

import (
	"fivem-installer/models"
	"fivem-installer/services"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"net/http"
	"os"
)

func main() {
	if err := godotenv.Load(); err != nil {
		// It's okay if .env doesn't exist in production if env vars are set otherwise
		// but we can log a warning to stdout
		println("No .env file found")
	}

	logger := services.NewDiscordLogger()
	logger.LogInfo("Server", "Backend started")

	scriptBytes, err := os.ReadFile("./scripts/setup.sh")
	if err != nil {
		logger.LogError("Startup", "Setup script not found!", err.Error())
		panic("Setup script not found!")
	}
	installerService := services.NewInstaller(string(scriptBytes), logger)

	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.POST("/api/install", func(c *gin.Context) {
		var req models.InstallRequest
		if err := c.BindJSON(&req); err != nil {
			logger.LogError("API", "Invalid JSON Request", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}
		result := installerService.Install(req)

		if !result.Success {
			c.JSON(http.StatusInternalServerError, result)
		} else {
			c.JSON(http.StatusOK, result)
		}
	})

	r.Run(":8080")
}
