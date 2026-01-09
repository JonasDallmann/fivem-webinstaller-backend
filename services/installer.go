package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fivem-installer/models"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Logger interface {
	LogInfo(section, msg string)
	LogError(section, msg, err string)
}

type Installer struct {
	ScriptContent string
	logger        Logger
}

func NewInstaller(scriptContent string, logger Logger) *Installer {
	return &Installer{
		ScriptContent: scriptContent,
		logger:        logger,
	}
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
			s.logger.LogError("SSH Connection", "Authentication Failed", errStr)
			return errorResponse("Login fehlgeschlagen", "AUTH_ERROR", "Das angegebene Passwort oder der Benutzername ist falsch.\nOriginal: "+errStr)
		}
		if strings.Contains(errStr, "refused") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
			s.logger.LogError("SSH Connection", "Connection Failed", errStr)
			return errorResponse("Server nicht erreichbar", "CONN_ERROR", "Verbindung fehlgeschlagen.\nOriginal: "+errStr)
		}
		s.logger.LogError("SSH Connection", "Unknown Error", errStr)
		return errorResponse("SSH Verbindungsfehler", "SSH_ERROR", errStr)
	}
	defer func(client *ssh.Client) {
		_ = client.Close()
	}(client)

	session, err := client.NewSession()
	if err != nil {
		s.logger.LogError("SSH Session", "Session Creation Failed", err.Error())
		return errorResponse("SSH Session Failed", "SESSION_ERROR", err.Error())
	}
	defer func(session *ssh.Session) {
		_ = session.Close()
	}(session)

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

	cleanScript := strings.ReplaceAll(s.ScriptContent, "\r", "")

	fullScript := envVars.String() + "\n" + cleanScript

	session.Stdin = bytes.NewBufferString(fullScript)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return errorResponse("Pipe Error", "PIPE_ERROR", err.Error())
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return errorResponse("Pipe Error", "PIPE_ERROR", err.Error())
	}

	if err := session.Start("bash"); err != nil {
		s.logger.LogError("SSH Start", "Command Start Failed", err.Error())
		return errorResponse("Script Start Failed", "START_ERROR", err.Error())
	}

	var outputBuffer bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()

			if strings.Contains(text, "JSON_START") || strings.Contains(text, "JSON_END") {
				outputBuffer.WriteString(text + "\n")
				continue
			}

			s.logger.LogInfo("REMOTE", text)
			outputBuffer.WriteString(text + "\n")
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text := scanner.Text()
			s.logger.LogInfo("REMOTE_ERR", text)
			outputBuffer.WriteString(text + "\n")
		}
	}()

	wg.Wait()
	err = session.Wait()

	fullOutput := outputBuffer.String()
	return s.parseScriptOutput(fullOutput, err)
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
		s.logger.LogError("Script Execution", "Invalid JSON Output", "SSH Error: "+errMsg)

		return models.InstallResponse{
			Success:   false,
			Error:     "Script Error / Invalid Output",
			ErrorCode: "SCRIPT_CRASH",
			RawLog:    output + "\n\nSSH Error: " + errMsg,
		}
	}

	jsonStr := output[startIndex+len(startTag) : endIndex]
	jsonStr = strings.TrimSpace(jsonStr)

	var resp models.InstallResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		s.logger.LogError("JSON Parsing", "Failed to unmarshal script response", jsonStr+"\nError: "+err.Error())
		return models.InstallResponse{
			Success: false,
			Error:   "JSON Parsing fehlgeschlagen",
			RawLog:  "Output: " + jsonStr + "\nError: " + err.Error(),
		}
	}

	if !resp.Success {
		s.logger.LogError("Installation Script", "Script reported failure", fmt.Sprintf("Error: %s\nCode: %s", resp.Error, resp.ErrorCode))
	} else {
		s.logger.LogInfo("Installation Script", "Installation successful on "+resp.TxAdminURL)
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