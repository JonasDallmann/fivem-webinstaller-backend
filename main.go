package main

import (
	"fivem-installer/models"
	"fivem-installer/services"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type Job struct {
	IP        string
	Status    string
	Logs      string
	Result    *models.InstallResponse
	CreatedAt time.Time
	mu        sync.RWMutex
}

var (
	jobStore = make(map[string]*Job)
	storeMu  sync.RWMutex
)

func getOrCreateJob(ip string) (*Job, bool) {
	storeMu.Lock()
	defer storeMu.Unlock()

	if job, exists := jobStore[ip]; exists {
		job.mu.RLock()
		if job.Status == "RUNNING" {
			job.mu.RUnlock()
			return job, true
		}
		job.mu.RUnlock()
	}

	newJob := &Job{
		IP:        ip,
		Status:    "RUNNING",
		Logs:      "Initialising Installer...\n",
		CreatedAt: time.Now(),
	}
	jobStore[ip] = newJob
	return newJob, false
}

func getJob(ip string) (*Job, bool) {
	storeMu.RLock()
	defer storeMu.RUnlock()
	job, exists := jobStore[ip]
	return job, exists
}

type SessionLogger struct {
	Job           *Job
	DiscordLogger *services.DiscordLogger
}

func (l *SessionLogger) appendLog(prefix, msg string) {
	l.Job.mu.Lock()
	defer l.Job.mu.Unlock()
	timestamp := time.Now().Format("15:04:05")
	l.Job.Logs += fmt.Sprintf("[%s] [%s] %s\n", timestamp, prefix, msg)
}

func (l *SessionLogger) LogInfo(section, msg string) {
	l.appendLog(section, msg)

	if section != "REMOTE" && section != "REMOTE_ERR" {
		titleWithIP := fmt.Sprintf("%s | 🖥️ %s", section, l.Job.IP)
		l.DiscordLogger.LogInfo(titleWithIP, msg)
	}
}

func (l *SessionLogger) LogError(section, msg, err string) {
	fullMsg := fmt.Sprintf("%s | Error: %s", msg, err)
	l.appendLog("ERROR/"+section, fullMsg)
	
	titleWithIP := fmt.Sprintf("%s | 🖥️ %s", section, l.Job.IP)

	l.DiscordLogger.LogError(titleWithIP, msg, err)
}

func main() {
	if err := godotenv.Load(); err != nil {
		println("No .env file found")
	}

	baseLogger := services.NewDiscordLogger()
	baseLogger.LogInfo("Server", "Backend started")

	scriptBytes, err := os.ReadFile("./scripts/setup.sh")
	if err != nil {
		baseLogger.LogError("Startup", "Setup script not found!", err.Error())
		panic("Setup script not found!")
	}
	scriptContent := string(scriptBytes)

	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.GET("/api/status", func(c *gin.Context) {
		host := c.Query("host")
		if host == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Host parameter missing"})
			return
		}

		job, exists := getJob(host)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "No job found for this host"})
			return
		}

		job.mu.RLock()
		defer job.mu.RUnlock()

		response := models.JobStatusResponse{
			IP:     job.IP,
			Status: job.Status,
			Logs:   job.Logs,
			Result: job.Result,
		}
		c.JSON(http.StatusOK, response)
	})

	r.POST("/api/install", func(c *gin.Context) {
		var req models.InstallRequest
		if err := c.BindJSON(&req); err != nil {
			baseLogger.LogError("API", "Invalid JSON Request", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		job, isBusy := getOrCreateJob(req.Host)
		if isBusy {
			c.JSON(http.StatusConflict, gin.H{
				"error":      "An installation is already running on this server.",
				"error_code": "BUSY",
			})
			return
		}

		go func(job *Job, request models.InstallRequest) {
			sessionLog := &SessionLogger{
					Job:           job,
					DiscordLogger: baseLogger,
				}

			installerService := services.NewInstaller(scriptContent, sessionLog)

			sessionLog.LogInfo("System", "Starting installation in the background...")
			result := installerService.Install(request)

			job.mu.Lock()
			job.Result = &result
			if result.Success {
				job.Status = "COMPLETED"
				job.Logs += "\n[System] Installation successfully done!\n"
			} else {
				job.Status = "ERROR"
				job.Logs += fmt.Sprintf("\n[System] Error: %s\n", result.Error)
			}
			job.mu.Unlock()

		}(job, req)

		c.JSON(http.StatusOK, gin.H{
			"status":  "started",
			"message": "Instalation started.",
			"ip":      req.Host,
		})
	})

	r.Run(":8080")
}