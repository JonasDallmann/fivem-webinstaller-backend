package services

import (
	"bytes"
	"encoding/json"
	"fivem-installer/models"
	"fmt"
	_ "log"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Installer struct {
	ScriptContent string
	Logger        *DiscordLogger
}

func NewInstaller(script string, logger *DiscordLogger) *Installer {
	return &Installer{ScriptContent: script, Logger: logger}
}

func (s *Installer) Install(req models.InstallRequest) models.InstallResponse {
	config := &ssh.ClientConfig{
		User: req.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(req.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unable to authenticate") {
			s.Logger.LogError("SSH Connection", "Authentication Failed", errStr)
			return errorResponse("Login fehlgeschlagen", "AUTH_ERROR", "Das angegebene Passwort oder der Benutzername ist falsch.\nOriginal: "+errStr)
		}
		if strings.Contains(errStr, "refused") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
			s.Logger.LogError("SSH Connection", "Connection Failed", errStr)
			return errorResponse("Server nicht erreichbar", "CONN_ERROR", "Verbindung fehlgeschlagen.\nOriginal: "+errStr)
		}
		s.Logger.LogError("SSH Connection", "Unknown Error", errStr)
		return errorResponse("SSH Verbindungsfehler", "SSH_ERROR", errStr)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		s.Logger.LogError("SSH Session", "Session Creation Failed", err.Error())
		return errorResponse("SSH Session Failed", "SESSION_ERROR", err.Error())
	}
	defer session.Close()

	var envVars strings.Builder

	if req.InstallMySQL {
		envVars.WriteString("export INSTALL_MYSQL=true\n")
	} else {
		envVars.WriteString("export INSTALL_MYSQL=false\n")
	}

	if req.ForceOverwrite {
		envVars.WriteString("export FORCE_OVERWRITE=true\n")
	} else {
		envVars.WriteString("export FORCE_OVERWRITE=false\n")
	}

	envVars.WriteString("export TERM=xterm\n")

	fullScript := envVars.String() + "\n" + s.ScriptContent

	session.Stdin = bytes.NewBufferString(fullScript)

	outputBytes, err := session.CombinedOutput("bash")

	outputStr := string(outputBytes)

	return s.parseScriptOutput(outputStr, err)
}

func (s *Installer) parseScriptOutput(output string, sshErr error) models.InstallResponse {
	startTag := "JSON_START"
	endTag := "JSON_END"

	startIndex := strings.Index(output, startTag)
	endIndex := strings.Index(output, endTag)

	if startIndex == -1 || endIndex == -1 || startIndex+len(startTag) >= endIndex {
		errMsg := "Keine gültige Antwort vom Installer-Script."
		if sshErr != nil {
			errMsg = sshErr.Error()
		}
		s.Logger.LogError("Script Execution", "Invalid JSON Output / Script Crash", output+"\n\nSSH Error: "+errMsg)
		return models.InstallResponse{
			Success:   false,
			Error:     "Script Error / SSH Error",
			ErrorCode: "SCRIPT_CRASH",
			RawLog:    output + "\n\nSSH Error: " + errMsg,
		}
	}

	jsonStr := output[startIndex+len(startTag) : endIndex]
	jsonStr = strings.TrimSpace(jsonStr)

	var resp models.InstallResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		s.Logger.LogError("JSON Parsing", "Failed to unmarshal script response", jsonStr+"\nError: "+err.Error())
		return models.InstallResponse{
			Success: false,
			Error:   "JSON Parsing fehlgeschlagen",
			RawLog:  "Output: " + jsonStr + "\nError: " + err.Error(),
		}
	}

	if !resp.Success {
		s.Logger.LogError("Installation Script", "Script reported failure", fmt.Sprintf("Error: %s\nCode: %s", resp.Error, resp.ErrorCode))
	} else {
		s.Logger.LogInfo("Installation Script", "Installation successful on "+resp.TxAdminURL)
	}

	return resp
}

func errorResponse(msg, code, log string) models.InstallResponse {
	return models.InstallResponse{
		Success:   false,
		Error:     msg,
		ErrorCode: code,
		RawLog:    log,
	}
}
