package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"fivem-installer/models"
	"fivem-installer/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/time/rate"
)

var validHostRegex = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

func isValidInput(input string) bool {
	if len(input) > 255 || len(input) < 3 {
		return false
	}
	return validHostRegex.MatchString(input)
}

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		c.Next()
	}
}

type Job struct {
	IP        string
	Status    string
	Logs      string
	Result    *models.InstallResponse
	CreatedAt time.Time
	mu        sync.RWMutex
}

type IPRateLimiter struct {
	ips map[string]*rate.Limiter
	mu  sync.Mutex
	r   rate.Limit
	b   int
}

var (
	jobStore = make(map[string]*Job)
	storeMu  sync.RWMutex
)

func NewIPRateLimiter() *IPRateLimiter {
	return &IPRateLimiter{
		ips: make(map[string]*rate.Limiter),
		r:   rate.Every(1 * time.Minute),
		b:   5,
	}
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}
	return limiter
}

var limiter = NewIPRateLimiter()

func rateLimitMiddleware(logger *services.DiscordLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.Request.Header.Get("X-Real-IP")
		if ip == "" {
			ip = c.ClientIP()
		}

		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			logger.LogError("Security", "Rate Limit Hit 🛡️", fmt.Sprintf("IP %s wurde temporär blockiert.", ip))
			fmt.Printf("[Security] Rate Limit Block: %s\n", ip)

			c.JSON(http.StatusTooManyRequests, models.InstallResponse{
				Success:   false,
				Error:     "Zu viele Versuche! Du bist für 1 Minute gesperrt.",
				ErrorCode: "RATE_LIMIT",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

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
		println("No .env file found (using system env vars)")
	}

	baseLogger := services.NewDiscordLogger()
	baseLogger.LogInfo("Server", "Backend started successfully\nThe FiveM API backend is now running.")

	scriptBytes, err := os.ReadFile("./scripts/setup.sh")
	if err != nil {
		baseLogger.LogError("Startup", "Setup script not found!", err.Error())
		panic("Setup script not found!")
	}
	scriptContent := string(scriptBytes)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(securityHeadersMiddleware())

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Real-IP")
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

		if !isValidInput(host) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Host format"})
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

	r.POST("/api/install", rateLimitMiddleware(baseLogger), func(c *gin.Context) {
		var req models.InstallRequest

		if err := c.BindJSON(&req); err != nil {
			baseLogger.LogError("API", "Invalid JSON Request", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		if !isValidInput(req.Host) {
			ip := c.ClientIP()
			baseLogger.LogError("Security", "Injection Attempt Blocked ⚠️", fmt.Sprintf("IP: %s | Input: %s", ip, req.Host))

			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Ungültiges Format für Host/IP. Nur Buchstaben, Zahlen, Punkte und Bindestriche erlaubt.",
			})
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

			sessionLog.LogInfo("System", "Starting installation process...")

			result := installerService.Install(request)
			services.LogTargetServer(request.Host, result.Success, request.InstallMySQL)

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
			"message": "Installation started.",
			"ip":      req.Host,
		})
	})

	fmt.Println("Server running on port 8080 (Secure Mode)")
	err = r.Run(":8080")
	if err != nil {
		return
	}
}
